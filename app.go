package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// StartupFile is the payload the frontend receives when Scribe opens a file.
//
// Wails v2 bindings support one return value plus an optional error. A Go
// method returning (path, content, error) is not supported by the binding
// layer, so path and content travel together in this small struct instead.
type StartupFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// App is deliberately small: it holds the Wails context (needed for runtime
// events) and the one piece of state the backend cares about, the current file
// path. The editor buffer itself lives in Svelte.
type App struct {
	ctx context.Context

	// mu protects ctx and filePath. macOS can deliver an "open file" event on
	// the Cocoa event loop while the frontend is reading the current path.
	mu       sync.RWMutex
	filePath string
}

// NewApp stores the path supplied on the command line. The path is not read
// here so that a missing or unreadable file can still produce a useful error
// in the UI instead of failing silently at startup.
func NewApp(startupFile string) *App {
	return &App{filePath: startupFile}
}

// startup is called by Wails once the application is ready. The context is
// saved because runtime.EventsEmit needs it.
func (a *App) startup(ctx context.Context) {
	a.mu.Lock()
	a.ctx = ctx
	a.mu.Unlock()
}

// GetStartupFile reads the file passed on the command line (or through macOS
// "Open With"). It returns an empty StartupFile when Scribe was launched with
// no file, which lets the frontend show an empty editor rather than an error.
//
// The frontend calls this once from onMount. File I/O stays in Go so the Svelte
// side never needs browser file APIs, which are not available in a Wails
// webview.
func (a *App) GetStartupFile() (StartupFile, error) {
	a.mu.RLock()
	path := a.filePath
	a.mu.RUnlock()

	if path == "" {
		return StartupFile{}, nil
	}

	// Stat first so a directory produces a clear message instead of the
	// platform-specific error that ReadFile would return.
	info, err := os.Stat(path)
	if err != nil {
		return StartupFile{}, fmt.Errorf("cannot open %s: %w", path, err)
	}
	if info.IsDir() {
		return StartupFile{}, fmt.Errorf("cannot open %s: it is a directory", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return StartupFile{}, fmt.Errorf("cannot read %s: %w", path, err)
	}

	return StartupFile{Path: path, Content: string(data)}, nil
}

// SaveFile writes the editor buffer back to disk. The frontend sends the path
// it is editing, so this method stays stateless and easy to test.
func (a *App) SaveFile(path string, content string) error {
	if path == "" {
		return errors.New("no file is open")
	}

	// os.WriteFile truncates an existing file and keeps its permissions. For a
	// new file, 0644 is a sensible default for a text document.
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("cannot save %s: %w", path, err)
	}

	return nil
}

// openFileFromOS is wired to Wails' macOS OnFileOpen hook. macOS sends "Open
// With" through an Apple Event instead of os.Args, so this callback gives the
// app the same behaviour as the command-line path.
//
// The event tells an already-open window to reload. If the window is still
// starting, the event is harmless: GetStartupFile picks up the new path when
// Svelte mounts.
func (a *App) openFileFromOS(path string) {
	a.mu.Lock()
	a.filePath = path
	ctx := a.ctx
	a.mu.Unlock()

	if ctx != nil {
		runtime.EventsEmit(ctx, "file:opened", path)
	}
}
