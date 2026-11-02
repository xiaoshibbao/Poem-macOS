package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"poem/internal/dialog"
	"poem/internal/launcher"
	"poem/internal/window"
)

//go:embed all:frontend/dist
var frontend embed.FS

// poem-helper.exe and LuaEntry.dll are built from this repository before
// `go build` (see scripts/build-macos.sh). The prebuilt game modules are not
// embedded; users place them in the modules folder shown in the app.
//
//go:embed assets/poem-helper.exe assets/LuaEntry.dll assets/appicon.png
var assets embed.FS

func mustRead(name string) []byte {
	b, err := assets.ReadFile("assets/" + name)
	if err != nil {
		log.Fatalf("embedded asset %s missing: %v", name, err)
	}
	return b
}

func main() {
	service := launcher.New(launcher.Modules{
		Helper: mustRead("poem-helper.exe"),
		Lua:    mustRead("LuaEntry.dll"),
	})
	var win *application.WebviewWindow
	app := application.New(application.Options{
		Name:        "Poem",
		Description: "Poem Launcher · 星穹铁道启动器（macOS）",
		Icon:        mustRead("appicon.png"),
		Services: []application.Service{
			application.NewService(service),
			application.NewService(&dialog.Service{}),
			application.NewService(&window.Service{}),
		},
		Assets: application.AssetOptions{Handler: application.AssetFileServerFS(frontend)},
		Mac:    application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
		// ⌘Q while a game runs would stop its proxy; hide the window instead.
		ShouldQuit: func() bool {
			if service.GetStatus().Running > 0 && win != nil {
				win.Minimise()
				return false
			}
			return true
		},
	})
	win = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:                      "Poem",
		Width:                      1280,
		Height:                     800,
		MinWidth:                   1100,
		MinHeight:                  720,
		BackgroundColour:           application.NewRGB(13, 18, 25),
		URL:                        "/",
		DevToolsEnabled:            false,
		DefaultContextMenuDisabled: true,
		ZoomControlEnabled:         false,
		// Native traffic lights over the app's own title area.
		Mac: application.MacWindow{
			TitleBar:                application.MacTitleBarHiddenInset,
			InvisibleTitleBarHeight: 0,
			Backdrop:                application.MacBackdropNormal,
		},
	})
	// Closing while a game is running only minimises, so the proxy stays alive.
	win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if service.GetStatus().Running > 0 {
			e.Cancel()
			win.Minimise()
		}
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
