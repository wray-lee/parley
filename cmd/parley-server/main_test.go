package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWebDir(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("PARLEY_WEB_DIR", "")

	if dir, err := webDir(); err != nil || dir != "" {
		t.Fatalf("no web-app: %q %v", dir, err)
	}
	if err := os.Mkdir("web-app", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("web-app", "index.html"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if dir, err := webDir(); err != nil || dir != "web-app" {
		t.Fatalf("web-app: %q %v", dir, err)
	}

	t.Setenv("PARLEY_WEB_DIR", "missing")
	if _, err := webDir(); err == nil {
		t.Fatal("an explicit missing PARLEY_WEB_DIR must fail loudly")
	}
}
