package launcher

import (
	"bytes"
	"fmt"
	"path/filepath"
	"poem/internal/gamelang"
)

func (s *Service) GetLanguageInfo(gamePath string) (gamelang.Info, error) {
	return gamelang.Inspect(filepath.Dir(gamePath))
}
func (s *Service) RestoreLanguage(gamePath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.sessions) > 0 {
		return fmt.Errorf("请先退出游戏，再恢复语言设置")
	}
	dir, err := dataDir()
	if err != nil {
		return err
	}
	return gamelang.Restore(filepath.Dir(gamePath), filepath.Join(dir, "language-backups"))
}
func applyLanguage(cfg Settings, dir string, running bool) error {
	if cfg.TextLanguage == "" && cfg.VoiceLanguage == "" {
		return nil
	}
	plan, err := gamelang.Prepare(filepath.Dir(cfg.GamePath), cfg.TextLanguage, cfg.VoiceLanguage)
	if err != nil {
		return fmt.Errorf("游戏语言修改失败：%w", err)
	}
	if bytes.Equal(plan.Original, plan.Patched) {
		return nil
	}
	if running {
		return fmt.Errorf("请先退出已运行的游戏，再应用不同的语言设置")
	}
	return gamelang.Apply(plan, filepath.Join(dir, "language-backups"))
}
