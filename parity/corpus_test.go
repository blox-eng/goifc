package parity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadPublicModels(t *testing.T) {
	for _, name := range Public {
		t.Run(name, func(t *testing.T) {
			f, err := Load(name)
			if err != nil {
				t.Fatalf("Load(%q) = %v", name, err)
			}
			if got := f.Len(); got == 0 {
				t.Errorf("Load(%q) parsed 0 instances", name)
			}
		})
	}
}

// A fixture truncated mid-stream must surface a wrapped error naming the model,
// not panic. Fixtures are binary blobs in git; a bad checkout produces exactly
// this.
func TestLoadCorruptFixtureErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "testdata"), 0o755); err != nil {
		t.Fatal(err)
	}
	good, err := os.ReadFile(filepath.Join(Dir(), "testdata", "ifcopenhouse.ifc.gz"))
	if err != nil {
		t.Fatal(err)
	}
	truncated := good[:len(good)/2]
	path := filepath.Join(dir, "testdata", "truncated.ifc.gz")
	if err := os.WriteFile(path, truncated, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err = loadGzip(path, "truncated")
	if err == nil {
		t.Fatal("loadGzip on a truncated fixture returned nil error")
	}
	if !strings.Contains(err.Error(), "truncated") {
		t.Errorf("error %q does not name the model", err)
	}
}

// A stale or wrong $GOIFC_PRIVATE_CORPUS must not fail a contributor's build.
func TestPrivateCorpusMissingIsSkipped(t *testing.T) {
	t.Setenv(envPrivateCorpus, filepath.Join(t.TempDir(), "does-not-exist"))
	for _, name := range Private {
		if Available(name) {
			t.Errorf("Available(%q) = true with a nonexistent private corpus", name)
		}
	}
	// The public models are unaffected.
	if !Available(Public[0]) {
		t.Errorf("Available(%q) = false; a bad private path broke the public corpus", Public[0])
	}
}
