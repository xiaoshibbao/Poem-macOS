package dialog

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type Service struct{}

// PickFile selects a file. start, when it exists, is the initial directory;
// hidden folders are shown because Wine prefixes usually live under ~/Library or ~/.wine.
func (f *Service) PickFile(filter, start string) (string, error) {
	dialog := application.Get().Dialog.OpenFile().
		CanChooseFiles(true).
		ResolvesAliases(true).
		ShowHiddenFiles(true).
		TreatsFilePackagesAsDirectories(filter == "wine")
	switch filter {
	case "exe":
		dialog.SetTitle("选择 StarRail.exe")
		dialog.AddFilter("Windows 程序 (*.exe)", "*.exe")
	case "lua":
		dialog.SetTitle("选择 Lua 脚本")
		dialog.AddFilter("Lua 脚本 (*.lua)", "*.lua")
	case "wine":
		dialog.SetTitle("选择 Wine 程序（如 bin/wine 或 bin/wine64）")
	}
	if info, err := os.Stat(start); start != "" && err == nil && info.IsDir() {
		dialog.SetDirectory(start)
	}
	if path, err := dialog.PromptForSingleSelection(); err == nil {
		return path, nil
	}
	return "", nil
}

// PickFolder selects a directory, such as a Wine prefix.
func (f *Service) PickFolder(start string) (string, error) {
	dialog := application.Get().Dialog.OpenFile().
		CanChooseFiles(false).
		CanChooseDirectories(true).
		CanCreateDirectories(false).
		ShowHiddenFiles(true)
	dialog.SetTitle("选择 Wine 容器（包含 drive_c 的目录）")
	if info, err := os.Stat(start); start != "" && err == nil && info.IsDir() {
		dialog.SetDirectory(start)
	}
	if path, err := dialog.PromptForSingleSelection(); err == nil {
		return path, nil
	}
	return "", nil
}

func (f *Service) OpenFolder(path string) (bool, string) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false, "failed to resolve absolute path: " + err.Error()
	}

	if info, err := os.Stat(absPath); err != nil || !info.IsDir() {
		return false, "directory not found: " + absPath
	}

	if err := exec.Command("open", absPath).Run(); err != nil {
		return false, err.Error()
	}
	return true, ""
}
