package window

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

type Service struct{}

func (a *Service) Close() (bool, string) {
	application.Get().Quit()
	return true, ""
}

func (a *Service) Minimize() (bool, string) {
	window := application.Get().Window.Current()
	if window == nil {
		return false, "not found window"
	}
	window.Minimise()
	return true, ""
}
