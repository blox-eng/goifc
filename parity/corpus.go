// Package parity holds goifc's public parity and coverage harness: a small
// corpus of redistributable IFC models, IfcOpenShell AABB oracles for them, and
// the two gates described in
// docs/design/2026-09-21-parity-and-coverage-harness.md.
//
// It is a separate module so that none of it reaches library users: Go prunes
// go.mod-bearing subdirectories from the parent's module zip.
package parity

import (
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"

	"github.com/blox-eng/goifc/step"
)

// envPrivateCorpus names a directory of uncompressed .ifc files that are not
// redistributable. When set, a Private model found there becomes loadable;
// public models ignore it entirely (see privatePath). Absent or wrong, every
// public gate still passes.
const envPrivateCorpus = "GOIFC_PRIVATE_CORPUS"

// Public lists the redistributable models committed under testdata. They are
// published samples: IfcOpenShell's own IfcOpenHouse, buildingSMART's Duplex
// Apartment common building model, and KIT Karlsruhe's FZK-Haus.
var Public = []string{"ifcopenhouse", "duplex_a", "fzk_haus"}

// Private lists models that exist only in $GOIFC_PRIVATE_CORPUS. They sharpen
// the gap ranking locally and never gate CI.
var Private = []string{"kb645", "office_a"}

// packageDir is resolved at init from this file's compile-time path, so both
// `go test ./...` (cwd = parity/) and cmd/coverage (cwd = parity/cmd/coverage/)
// find testdata. The harness always runs from source, never from a module zip.
var packageDir = func() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("parity: cannot resolve package directory")
	}
	return filepath.Dir(file)
}()

// Dir returns the parity package directory.
func Dir() string { return packageDir }

// privatePath returns the uncompressed path for name under
// $GOIFC_PRIVATE_CORPUS, and whether it is readable.
//
// Only Private names are looked up there. A public model always loads its
// committed fixture, even when a like-named file sits in the private corpus:
// otherwise a stray duplex_a.ifc silently redefines a public model, which fails
// `cmd/coverage -check` for the contributor holding it and — worse — makes the
// remediation that failure names (`make parity-report`, `make parity-baseline`)
// write non-redistributable measurements into the published page as if they
// were the public corpus's own figures.
func privatePath(name string) (string, bool) {
	if !slices.Contains(Private, name) {
		return "", false
	}
	root := os.Getenv(envPrivateCorpus)
	if root == "" {
		return "", false
	}
	p := filepath.Join(root, name+".ifc")
	if st, err := os.Stat(p); err != nil || st.IsDir() {
		return "", false
	}
	return p, true
}

// fixturePath returns the committed gzipped path for name, and whether it exists.
func fixturePath(name string) (string, bool) {
	p := filepath.Join(packageDir, "testdata", name+".ifc.gz")
	if st, err := os.Stat(p); err != nil || st.IsDir() {
		return "", false
	}
	return p, true
}

// Available reports whether name can be loaded: a committed fixture, or a
// private model whose source is readable.
func Available(name string) bool {
	if _, ok := privatePath(name); ok {
		return true
	}
	_, ok := fixturePath(name)
	return ok
}

// Load parses the named model. For a Private name a readable private source is
// the only source; a Public name always comes from its committed fixture, so
// what the gates and the published page measure does not depend on which files
// happen to be on the machine.
func Load(name string) (*step.File, error) {
	if p, ok := privatePath(name); ok {
		return loadPlain(p, name)
	}
	p, ok := fixturePath(name)
	if !ok {
		return nil, fmt.Errorf("parity: model %q not available (no fixture, and $%s does not provide it)", name, envPrivateCorpus)
	}
	return loadGzip(p, name)
}

func loadPlain(path, name string) (*step.File, error) {
	r, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("parity: open %s: %w", name, err)
	}
	defer func() { _ = r.Close() }()
	f, err := step.Parse(r)
	if err != nil {
		return nil, fmt.Errorf("parity: parse %s: %w", name, err)
	}
	return f, nil
}

func loadGzip(path, name string) (*step.File, error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("parity: open %s: %w", name, err)
	}
	defer func() { _ = fh.Close() }()
	zr, err := gzip.NewReader(fh)
	if err != nil {
		return nil, fmt.Errorf("parity: gunzip %s: %w", name, err)
	}
	defer func() { _ = zr.Close() }()
	f, err := step.Parse(zr)
	if err != nil {
		return nil, fmt.Errorf("parity: parse %s: %w", name, err)
	}
	return f, nil
}
