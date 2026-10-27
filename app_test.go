package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStartupFileFromArgs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.md")
	if err := os.WriteFile(path, []byte("# hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := startupFileFromArgs([]string{"scribe", "-psn_0_123", path}); got != path {
		t.Fatalf("got %q, want %q", got, path)
	}
	if got := startupFileFromArgs([]string{"scribe"}); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestGetAndSaveFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.md")
	if err := os.WriteFile(path, []byte("# hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	app := NewApp(path)
	file, err := app.GetStartupFile()
	if err != nil {
		t.Fatal(err)
	}
	if file.Path != path || file.Content != "# hello\n" {
		t.Fatalf("unexpected file: %#v", file)
	}

	if err := app.SaveFile(path, "# changed\n"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "# changed\n" {
		t.Fatalf("got %q", data)
	}
}

func TestMissingFileError(t *testing.T) {
	app := NewApp(filepath.Join(t.TempDir(), "missing.md"))
	_, err := app.GetStartupFile()
	if err == nil || !strings.Contains(err.Error(), "cannot open") {
		t.Fatalf("expected a clear open error, got %v", err)
	}
}
