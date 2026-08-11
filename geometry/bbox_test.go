package geometry

import (
	"fmt"
	"strings"
	"testing"

	"github.com/blox-eng/goifc/step"
)

// TestCollectPoints_DepthBound proves collectPoints' walk terminates on a
// deeply-nested (but acyclic, so the seen-set alone doesn't help) forward-ref
// chain instead of recursing until it stack-overflows the worker — the same
// untrusted-input hardening maxMapDepth already provides for mapped items.
// The chain is built well past maxWalkDepth; a real IFC representation
// subgraph is only a handful of levels deep.
func TestCollectPoints_DepthBound(t *testing.T) {
	const chainLen = maxWalkDepth + 1000

	var b strings.Builder
	b.WriteString("ISO-10303-21;\nHEADER;\n")
	b.WriteString("FILE_DESCRIPTION((''),'2;1');\n")
	b.WriteString("FILE_NAME('deep.ifc','',(''),(''),'','','');\n")
	b.WriteString("FILE_SCHEMA(('IFC4'));\n")
	b.WriteString("ENDSEC;\nDATA;\n")

	// #1 is the root; each IFCLOCALPLACEMENT's first (PlacementRelTo) attribute
	// chains to the next instance, terminating at a real IFCCARTESIANPOINT far
	// past maxWalkDepth. collectPoints doesn't validate entity semantics, so a
	// simple ref chain via a real IFC entity type exercises the same forward-ref
	// walk as a pathological representation subgraph would.
	for id := 1; id <= chainLen; id++ {
		fmt.Fprintf(&b, "#%d=IFCLOCALPLACEMENT(#%d,$);\n", id, id+1)
	}
	fmt.Fprintf(&b, "#%d=IFCCARTESIANPOINT((1.,2.,3.));\n", chainLen+1)
	b.WriteString("ENDSEC;\nEND-ISO-10303-21;\n")

	f, err := step.ParseBytes([]byte(b.String()))
	if err != nil {
		t.Fatalf("parse deep chain: %v", err)
	}
	root, ok := f.ByID(1)
	if !ok {
		t.Fatal("root instance #1 missing")
	}

	pts := collectPoints(root)

	// The point past maxWalkDepth must NOT be reached — proves the bound
	// actually stopped the walk (rather than the chain just happening to be
	// short enough to fully traverse).
	if len(pts) != 0 {
		t.Errorf("collectPoints found %d points, want 0 (the real point sits beyond maxWalkDepth=%d)", len(pts), maxWalkDepth)
	}
}
