package ifc

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/blox-eng/goifc/step"
)

// FuzzAssemble drives the WHOLE untrusted-input path — parse, semantic extract,
// tessellate, derive quantities, write GLB — because that is the path a
// third-party IFC file actually takes through a consumer. The geometry stage is
// where the hand-reasoned recursion guards live (maxMapDepth, maxWalkDepth,
// maxApproxLadder), and their own comments say the failure they prevent is a
// stack overflow that recover() cannot catch and that takes down the worker
// process. A guard nobody exercises adversarially is a guard nobody has tested.
//
// The contract asserted is deliberately weak — no panic, no hang, no unbounded
// recursion, and an error rather than a half-built result. Garbage in may
// legitimately yield an empty scene; it must never yield a crash.
func FuzzAssemble(f *testing.F) {
	for _, g := range []string{
		"testdata/*.ifc",
		"geometry/testdata/synthetic/*.ifc",
		"model/testdata/synthetic/*.ifc",
		"step/testdata/*.ifc",
	} {
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

	f.Fuzz(func(t *testing.T, src []byte) {
		file, err := step.ParseBytes(src)
		if err != nil {
			return
		}
		a, err := Assemble(file)
		if err != nil {
			if a != nil {
				t.Fatalf("Assemble returned both a result and an error: %v", err)
			}
			return
		}
		if a.Result == nil || a.Scene == nil {
			t.Fatal("Assemble returned a nil Result or Scene without an error")
		}
		// An element carrying a mesh must carry a well-formed one: Tris index
		// Verts by vertex triple, so an out-of-range index is a corrupt mesh
		// that would fault whatever renders or measures it downstream.
		for _, e := range a.Scene.Elements {
			nv := uint32(len(e.Verts) / 3)
			for _, idx := range e.Tris {
				if idx >= nv {
					t.Fatalf("element %s (%s): triangle index %d out of range for %d vertices",
						e.GlobalID, e.Source, idx, nv)
				}
			}
		}
		if err := a.Scene.WriteGLB(io.Discard); err != nil {
			t.Fatalf("WriteGLB on an assembled scene: %v", err)
		}
	})
}
