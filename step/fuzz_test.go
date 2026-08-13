package step

import (
	"os"
	"path/filepath"
	"testing"
)

// seedFromFixtures adds every .ifc file under the given globs to the corpus, so
// fuzzing starts from structurally valid STEP rather than random bytes.
//
// Patterns are plain [filepath.Glob], which has NO globstar: `testdata/**/*.ifc`
// is not a recursive match, it is an ordinary segment pattern that quietly
// resolves to nothing. Spell each directory out, and fail loudly on a pattern
// matching no file — a silently empty seed corpus turns a fuzz target into
// decoration that still reports PASS.
func seedFromFixtures(f *testing.F, globs ...string) {
	f.Helper()
	for _, g := range globs {
		paths, err := filepath.Glob(g)
		if err != nil {
			f.Fatalf("glob %s: %v", g, err)
		}
		if len(paths) == 0 {
			f.Fatalf("seed glob %q matched no files — corpus would be thinner than it reads", g)
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
	// Every valid fixture in the repo, not just this package's four: the parser
	// is what every other package's fixture had to survive first, so they are
	// all legitimate seeds and they cost nothing.
	seedFromFixtures(f,
		"testdata/*.ifc",
		"../testdata/*.ifc",
		"../geometry/testdata/synthetic/*.ifc",
		"../model/testdata/synthetic/*.ifc",
	)
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
