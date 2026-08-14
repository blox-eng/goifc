package ifc_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// internalRefs are Blox-internal artifacts that must not appear anywhere in
// this library's source. goifc is a public Go module; a reader outside Blox
// cannot open import_emit.py, does not run Temporal, and has never heard of
// the service called "flow". References to ifcopenshell are deliberately NOT
// listed — that is a public project this library ports from, and naming it is
// how a reader checks our behaviour against the reference.
var internalRefs = []string{
	"import_emit",
	"semantic_oracle",
	"Temporal",
	"flow's",
	"flow import contract",
	"flow workflow",
}

// thisFile is skipped: it necessarily contains every banned string above.
const thisFile = "internal_refs_test.go"

func TestNoInternalReferences(t *testing.T) {
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Nothing under .git or testdata is published documentation.
			if name := d.Name(); name == ".git" || name == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || filepath.Base(path) == thisFile {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(src), "\n") {
			for _, ref := range internalRefs {
				if strings.Contains(line, ref) {
					t.Errorf("%s:%d: Blox-internal reference %q in a public library:\n\t%s",
						path, i+1, ref, strings.TrimSpace(line))
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the module: %v", err)
	}
}
