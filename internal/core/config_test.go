package core

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/errata-ai/vale/v3/internal/system"
)

var testData = filepath.Join("..", "..", "testdata")

func TestInitCfg(t *testing.T) {
	cfg, err := NewConfig(&CLIFlags{})
	if err != nil {
		t.Fatal(err)
	}
	path := cfg.StylesPath()

	// In v3.0, these should have defaults.
	if path == "" {
		t.Fatal("StylesPath is empty")
	} else if !system.IsDir(path) {
		t.Fatalf("%s is not a directory", path)
	}
}

func TestGetIgnores(t *testing.T) {
	found, err := IgnoreFiles(filepath.Join(testData, "styles"))
	if err != nil {
		t.Fatal(err)
	} else if len(found) != 2 {
		t.Fatalf("Expected 2 ignore files, found %d", len(found))
	}
}

// TestFindAsset tests that we find an asset on the user-specified StylesPath.
//
// This was the standard behavior in v2.0.
func TestFindAsset(t *testing.T) {
	cfg, err := NewConfig(&CLIFlags{})
	if err != nil {
		t.Fatal(err)
	}
	cfg.AddStylesPath(filepath.Join(testData, "styles"))

	found := FindAsset(cfg, "line.tmpl")
	if found == "" {
		t.Fatal("Expected to find line.tmpl")
	}

	found = FindAsset(cfg, "notfound")
	if found != "" {
		t.Fatal("Expected to not find notfound")
	}
}

// TestFindAssetDefault tests that we find an asset on the default StylesPath
// when there's no user-specified StylesPath.
func TestFindAssetDefault(t *testing.T) {
	cfg, err := NewConfig(&CLIFlags{})
	if err != nil {
		t.Fatal(err)
	}

	expected, err := DefaultStylesPath()
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(expected, "tmp.tmpl")

	err = os.WriteFile(target, []byte{}, 0600)
	if err != nil {
		t.Fatal("Failed to create file", err)
	}

	found := FindAsset(cfg, "tmp.tmpl")
	if found == "" {
		t.Fatal("Expected to find 'tmp.tmpl', got empty")
	}

	found = FindAsset(cfg, "notfound")
	if found != "" {
		t.Fatal("Expected to not find 'notfound', got", found)
	}

	err = os.Remove(target)
	if err != nil {
		t.Fatal(err)
	}
}

// TestFallbackToDefault tests that we find an asset on the default StylesPath
// when there's a user-specified StylesPath, but the asset isn't there.
func TestFallbackToDefault(t *testing.T) {
	cfg, err := NewConfig(&CLIFlags{})
	if err != nil {
		t.Fatal(err)
	}

	local := filepath.Join(testData, "styles")
	cfg.AddStylesPath(local)

	expected, err := DefaultStylesPath()
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(expected, "tmp.tmpl")

	err = os.WriteFile(target, []byte{}, 0600)
	if err != nil {
		t.Fatal("Failed to create file", err)
	}

	found := FindAsset(cfg, "tmp.tmpl")
	if found == "" {
		t.Fatal("Expected to find 'tmp.tmpl', got empty", cfg.Paths)
	}

	err = os.Remove(target)
	if err != nil {
		t.Fatal(err)
	}
}
