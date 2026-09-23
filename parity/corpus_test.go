package parity

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blox-eng/goifc/step"
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

// A like-named file in the private corpus must not redefine a PUBLIC model.
// If it did, `cmd/coverage -check` would fail for the contributor holding it,
// and the remediation that failure names (`make parity-report`,
// `make parity-baseline`) would write non-redistributable measurements into the
// published page as the public corpus's own figures.
func TestPrivateCorpusCannotShadowPublicModel(t *testing.T) {
	dir := t.TempDir()
	// A real IFC file, but the wrong one: fzk_haus's bytes under duplex_a's name.
	other, err := loadFixtureBytes("fzk_haus")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "duplex_a.ifc"), other, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(envPrivateCorpus, dir)

	if p, ok := privatePath("duplex_a"); ok {
		t.Errorf("privatePath(\"duplex_a\") = %q, true; a public model must ignore the private corpus", p)
	}
	f, err := Load("duplex_a")
	if err != nil {
		t.Fatalf("Load(\"duplex_a\") = %v", err)
	}
	// The committed duplex_a fixture is far larger than fzk_haus; if the
	// shadow had won, the instance count would be fzk_haus's.
	shadow, err := step.Parse(bytes.NewReader(other))
	if err != nil {
		t.Fatal(err)
	}
	if f.Len() == shadow.Len() {
		t.Errorf("Load(\"duplex_a\") parsed %d instances, the same as the shadowing file — the private corpus overrode a public model", f.Len())
	}
}

// loadFixtureBytes returns a committed fixture's decompressed contents.
func loadFixtureBytes(name string) ([]byte, error) {
	p, ok := fixturePath(name)
	if !ok {
		return nil, fmt.Errorf("no fixture for %q", name)
	}
	rc, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	zr, err := gzip.NewReader(rc)
	if err != nil {
		return nil, err
	}
	defer func() { _ = zr.Close() }()
	return io.ReadAll(zr)
}
