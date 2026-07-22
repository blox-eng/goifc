package step

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func realPath() string { return filepath.Join("testdata", "real", "kb645.ifc") }

// TestParse_KB645 is the #2207 acceptance criterion: tokenize the real ~28 MB
// ArchiCAD export in-process, build a navigable graph with forward+inverse refs,
// and record parse time + peak heap. It skips when the (proprietary, gitignored)
// fixture is absent so CI stays green without it.
func TestParse_KB645(t *testing.T) {
	path := realPath()
	if _, err := os.Stat(path); err != nil {
		t.Skip("kb645.ifc not present; skipping real-file acceptance")
	}

	var m0, m1 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m0)

	f, err := ParseFile(path)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	runtime.ReadMemStats(&m1)

	if f.SchemaID() != "IFC2X3" {
		t.Fatalf("schema %q want IFC2X3", f.SchemaID())
	}
	if f.Len() != 528228 {
		t.Fatalf("instance count %d want 528228", f.Len())
	}

	// forward ref chain: #33 IFCOWNERHISTORY -> #28 IFCPERSONANDORGANIZATION
	oh, ok := f.ByID(33)
	if !ok || oh.Type() != "IFCOWNERHISTORY" {
		t.Fatalf("#33 = %v", oh)
	}
	if po, ok := oh.Ref(0); !ok || po.ID() != 28 {
		t.Fatalf("#33.arg0 -> %v want #28", po)
	}

	// derived '*' on IfcSiUnit #34
	if u, ok := f.ByID(34); ok {
		if a0, _ := u.Get(0); a0.Kind != KindDerived {
			t.Fatalf("#34.arg0 %+v want derived", a0)
		}
	}

	// inverse index is populated (load-bearing for the spatial-tree child)
	totalInv := 0
	for inst := range f.All() {
		totalInv += f.TotalInverses(inst)
	}
	if totalInv == 0 {
		t.Fatal("no inverse references built")
	}

	if len(f.Warnings()) != 0 {
		t.Logf("parse warnings (%d): first = %s", len(f.Warnings()), f.Warnings()[0])
	}
	t.Logf("kb645: %d instances, %d entity types, %d total inverse edges, heap +%d MB",
		f.Len(), len(f.byType), totalInv, (m1.HeapAlloc-m0.HeapAlloc)/(1024*1024))
}

// BenchmarkParse_KB645 measures parse wall-time and allocations on the real file.
func BenchmarkParse_KB645(b *testing.B) {
	path := realPath()
	if _, err := os.Stat(path); err != nil {
		b.Skip("kb645.ifc not present")
	}
	src, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := ParseBytes(src); err != nil {
			b.Fatal(err)
		}
	}
}
