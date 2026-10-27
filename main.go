package main

import (
	"embed"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

// The compiled Svelte app is embedded into the Go binary. Wails builds the
// frontend before compiling Go, so frontend/dist exists at build time.
//
//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// os.Args is how Windows and Linux deliver "Open With" file paths. macOS
	// uses the OnFileOpen callback below instead.
	app := NewApp(startupFileFromArgs(os.Args))

	err := wails.Run(&options.App{
		Title:     "Scribe",
		Width:     1100,
		Height:    800,
		MinWidth:  480,
		MinHeight: 320,

		AssetServer: &assetserver.Options{Assets: assets},

		// Match the editor background so resizing does not flash white.
		BackgroundColour: &options.RGBA{R: 22, G: 24, B: 29, A: 255},

		OnStartup: app.startup,

		// macOS does not put the opened file in os.Args. Wails receives the
		// Apple Event and forwards the path here.
		Mac: &mac.Options{
			OnFileOpen: app.openFileFromOS,
		},

		// Binding the App struct is what generates the JavaScript functions in
		// frontend/wailsjs/go/main/App.js. The frontend imports those generated
		// functions instead of calling window.go by hand.
		Bind: []interface{}{app},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}

// startupFileFromArgs returns the first argument that looks like a file path.
//
// Flags are skipped because macOS may add arguments such as -psn_0_12345, and
// development tooling can add its own flags. The path is made absolute so a
// later save does not depend on the process working directory.
//
// We intentionally do not check whether the file exists here. Passing the path
// through lets GetStartupFile return a readable error to the UI for missing
// files or permission problems.
func startupFileFromArgs(args []string) string {
	for _, arg := range args[1:] {
		if arg == "" || strings.HasPrefix(arg, "-") {
			continue
		}

		if absolute, err := filepath.Abs(arg); err == nil {
			return absolute
		}
		return arg
	}

	return ""
}
