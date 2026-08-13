package step

import (
	"os"
	"path/filepath"
	"testing"
)

// seedFromFixtures adds every .ifc file under the given globs to the corpus, so
// fuzzing starts from structurally valid STEP rather than random bytes.
func seedFromFixtures(f *testing.F, globs ...string) {
	f.Helper()
	for _, g := range globs {
		paths, err := filepath.Glob(g)
		if err != nil {
			f.Fatalf("glob %s: %v", g, err)
		}
		for _, p := range paths {
			b, err := os.ReadFile(p)
			if err != nil {
				f.Fatalf("read %s: %v", p, err)
			}
			f.Add(b)
		}
	}
}

// FuzzParseBytes asserts the parser's contract against UNTRUSTED input: an IFC
// file arrives from a third party (a modeller, a client, an upload), so
// malformed or adversarial STEP must produce an error, never a panic and never
// an unbounded walk. The parser's depth guards (maxWalkDepth, maxMapDepth,
// maxApproxLadder) are hand-reasoned; this is what tests that they hold.
//
// The post-parse traversal matters as much as the parse: a File that parses but
// whose reference graph panics on walk is the same crash, one call later.
func FuzzParseBytes(f *testing.F) {
	seedFromFixtures(f, "testdata/*.ifc", "testdata/**/*.ifc", "../testdata/*.ifc")
	f.Add([]byte("ISO-10303-21;\nHEADER;\nENDSEC;\nDATA;\nENDSEC;\nEND-ISO-10303-21;\n"))
	f.Add([]byte(""))

	f.Fuzz(func(t *testing.T, src []byte) {
		file, err := ParseBytes(src)
		if err != nil {
			if file != nil {
				t.Fatalf("ParseBytes returned both a file and an error: %v", err)
			}
			return
		}
		if file == nil {
			t.Fatal("ParseBytes returned nil file and nil error")
		}
		for inst := range file.All() {
			for _, a := range inst.Args() {
				a.Walk(func(Value) {})
			}
		}
	})
}
