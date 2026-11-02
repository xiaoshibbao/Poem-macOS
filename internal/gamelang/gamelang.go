package gamelang

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"poem/internal/gamelang/assetmeta"
	"poem/internal/gamelang/excel"
	"poem/internal/gamever"
	"strings"
)

type Service struct{}
type Info struct {
	Text    string `json:"text"`
	Voice   string `json:"voice"`
	Version string `json:"version"`
}
type Plan struct {
	File     string
	Offset   int64
	Original []byte
	Patched  []byte
	Index    string
}
type Backup struct {
	File     string
	Offset   int64
	Original []byte
	Patched  []byte
}

func isValidLang(lang string) bool {
	return lang == "cn" || lang == "en" || lang == "jp" || lang == "kr"
}
func readTable(gameDir string) (*excel.ExcelLanguage, []excel.LanguageRow, string, error) {
	folder := filepath.Join(gameDir, "StarRail_Data", "StreamingAssets", "DesignData", "Windows")
	hash, err := assetmeta.GetIndexHash(folder)
	if err != nil {
		return nil, nil, "", err
	}
	index, err := assetmeta.DesignIndexFromBytes(folder, hash)
	if err != nil {
		return nil, nil, "", err
	}
	data, file, err := index.FindAllowedLanguage()
	if err != nil {
		return nil, nil, "", err
	}
	if data.Size == 0 || data.Size > 1024*1024 {
		return nil, nil, "", fmt.Errorf("unsupported language table size: %d", data.Size)
	}
	table := excel.NewExcelLanguage(folder, &data, &file)
	rows, err := table.Parse()
	return table, rows, hash, err
}
func Inspect(gameDir string) (Info, error) {
	_, rows, _, err := readTable(gameDir)
	if err != nil {
		return Info{}, err
	}
	version, err := gamever.ParseBinaryVersion(filepath.Join(gameDir, "StarRail_Data", "StreamingAssets", "BinaryVersion.bytes"))
	if err != nil {
		return Info{}, err
	}
	area := "os"
	if strings.Contains(version.Name, "CN") {
		area = "cn"
	}
	info := Info{Version: version.String()}
	for _, row := range rows {
		if row.Area == nil || *row.Area != area || row.DefaultLanguage == nil {
			continue
		}
		if row.Type == nil {
			info.Text = *row.DefaultLanguage
		} else if *row.Type == 1 {
			info.Voice = *row.DefaultLanguage
		}
	}
	if !isValidLang(info.Text) || !isValidLang(info.Voice) {
		return Info{}, fmt.Errorf("language defaults not found")
	}
	return info, nil
}
func Prepare(gameDir, text, voice string) (Plan, error) {
	if text != "" && !isValidLang(text) || voice != "" && !isValidLang(voice) {
		return Plan{}, fmt.Errorf("unsupported language")
	}
	table, rows, index, err := readTable(gameDir)
	if err != nil {
		return Plan{}, err
	}
	changes := 0
	for i := range rows {
		row := &rows[i]
		if row.Area == nil || (*row.Area != "cn" && *row.Area != "os") {
			continue
		}
		lang := ""
		if row.Type == nil {
			lang = text
		} else if *row.Type == 1 {
			lang = voice
		}
		if lang == "" {
			continue
		}
		value := lang
		row.DefaultLanguage = &value
		row.LanguageList = []string{value}
		changes++
	}
	if changes == 0 {
		return Plan{}, fmt.Errorf("no matching language rows")
	}
	data, err := table.Unmarshal(rows)
	if err != nil {
		return Plan{}, err
	}
	if len(data) > int(table.ExcelDataEntry.Size) {
		return Plan{}, fmt.Errorf("language table exceeds reserved size: %d > %d", len(data), table.ExcelDataEntry.Size)
	}
	file := filepath.Join(table.AssetFolder, table.ExcelFileEntry.FileByteName+".bytes")
	offset := int64(table.ExcelDataEntry.Offset)
	original, err := readSegment(file, offset, int(table.ExcelDataEntry.Size))
	if err != nil {
		return Plan{}, err
	}
	patched := make([]byte, len(original))
	copy(patched, data)
	return Plan{File: file, Offset: offset, Original: original, Patched: patched, Index: index}, nil
}
func readSegment(file string, offset int64, size int) ([]byte, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if offset < 0 || size <= 0 || size > 1024*1024 || offset+int64(size) > info.Size() {
		return nil, fmt.Errorf("language table out of file bounds")
	}
	b := make([]byte, size)
	_, err = f.ReadAt(b, offset)
	return b, err
}
func writeSegment(file string, offset int64, expected, data []byte) error {
	if len(expected) != len(data) {
		return fmt.Errorf("language patch size mismatch")
	}
	current, err := readSegment(file, offset, len(expected))
	if err != nil {
		return err
	}
	if !bytes.Equal(current, expected) {
		return fmt.Errorf("language table changed; reload before applying")
	}
	f, err := os.OpenFile(file, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	n, err := f.WriteAt(data, offset)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	if err != nil {
		_, _ = f.WriteAt(expected, offset)
		return err
	}
	return f.Sync()
}
func backupPath(plan Plan, dir string) string {
	hash := sha256.Sum256([]byte(strings.ToLower(plan.File) + fmt.Sprintf(":%d:%d:", plan.Offset, len(plan.Original)) + plan.Index))
	return filepath.Join(dir, hex.EncodeToString(hash[:16])+".json")
}
func Apply(plan Plan, backupDir string) error {
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return err
	}
	path := backupPath(plan, backupDir)
	backup := Backup{File: plan.File, Offset: plan.Offset, Original: plan.Original, Patched: plan.Patched}
	if b, err := os.ReadFile(path); err == nil {
		var old Backup
		if err = json.Unmarshal(b, &old); err != nil {
			return err
		}
		if old.File != plan.File || old.Offset != plan.Offset || len(old.Original) != len(plan.Original) {
			return fmt.Errorf("invalid language backup")
		}
		if !bytes.Equal(plan.Original, old.Patched) && !bytes.Equal(plan.Original, old.Original) {
			return fmt.Errorf("language table changed outside Poem; preserve existing backup")
		}
		backup.Original = old.Original
	} else if !os.IsNotExist(err) {
		return err
	}
	b, err := json.Marshal(backup)
	if err != nil {
		return err
	}
	if err = os.WriteFile(path+".tmp", b, 0600); err != nil {
		return err
	}
	if err = os.Rename(path+".tmp", path); err != nil {
		return err
	}
	return writeSegment(plan.File, plan.Offset, plan.Original, plan.Patched)
}
func Restore(gameDir, backupDir string) error {
	table, _, index, err := readTable(gameDir)
	if err != nil {
		return err
	}
	file := filepath.Join(table.AssetFolder, table.ExcelFileEntry.FileByteName+".bytes")
	original, err := readSegment(file, int64(table.ExcelDataEntry.Offset), int(table.ExcelDataEntry.Size))
	if err != nil {
		return err
	}
	plan := Plan{File: file, Offset: int64(table.ExcelDataEntry.Offset), Original: original, Index: index}
	b, err := os.ReadFile(backupPath(plan, backupDir))
	if err != nil {
		return err
	}
	var backup Backup
	if err = json.Unmarshal(b, &backup); err != nil {
		return err
	}
	if backup.File != file || backup.Offset != plan.Offset || len(backup.Original) != len(original) {
		return fmt.Errorf("backup does not match this game version")
	}
	if bytes.Equal(original, backup.Original) {
		return nil
	}
	if !bytes.Equal(original, backup.Patched) {
		return fmt.Errorf("language table modified externally; refusing to overwrite")
	}
	return writeSegment(file, plan.Offset, original, backup.Original)
}

// Keep the upstream bindings available for callers using the original service.
func (l *Service) GetLanguage(path string) (bool, string, string, string) {
	info, err := Inspect(filepath.Dir(filepath.Dir(path)))
	if err != nil {
		return false, "", "", err.Error()
	}
	return true, info.Text, info.Voice, ""
}
func (l *Service) SetLanguage(path, text, voice string) (bool, string) {
	gameDir := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(path))))
	plan, err := Prepare(gameDir, text, voice)
	if err != nil {
		return false, err.Error()
	}
	if err = Apply(plan, filepath.Join(gameDir, "Poem-language-backups")); err != nil {
		return false, err.Error()
	}
	return true, "success"
}
