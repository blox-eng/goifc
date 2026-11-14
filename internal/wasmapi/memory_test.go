//go:build js && wasm

package wasmapi

import (
	"bytes"
	"fmt"
	"os"
	"runtime"
	"testing"
)

// bigIFC writes a synthetic IFC of about size bytes: one storey holding
// repeated walls, each with its own placement, extrusion and property set.
func bigIFC(size int) []byte {
	var b bytes.Buffer
	b.Grow(size + 1<<20)
	b.WriteString("ISO-10303-21;\nHEADER;\nFILE_DESCRIPTION((''),'2;1');\nFILE_NAME('big.ifc','',(''),(''),'','','');\nFILE_SCHEMA(('IFC4'));\nENDSEC;\nDATA;\n" +
		"#1=IFCSIUNIT(*,.LENGTHUNIT.,$,.METRE.);\n#2=IFCUNITASSIGNMENT((#1));\n" +
		"#5=IFCPROJECT('0GUIDproject00000000005',$,'Proj',$,$,$,$,$,#2);\n" +
		"#6=IFCBUILDINGSTOREY('0GUIDstorey000000000006',$,'Ground',$,$,$,$,$,.ELEMENT.,0.);\n" +
		"#7=IFCRELAGGREGATES('0GUIDra0000000000000007',$,$,$,#5,(#6));\n")
	var walls []int
	id := 100
	for n := 0; b.Len() < size-64*len(walls)-4096; n++ {
		x, y := float64(n%500)*4, float64(n/500)*4
		fmt.Fprintf(&b, "#%d=IFCCARTESIANPOINT((%g,%g,0.));\n#%d=IFCAXIS2PLACEMENT3D(#%d,$,$);\n#%d=IFCLOCALPLACEMENT($,#%d);\n", id, x, y, id+1, id, id+2, id+1)
		fmt.Fprintf(&b, "#%d=IFCCARTESIANPOINT((0.,0.));\n#%d=IFCAXIS2PLACEMENT2D(#%d,$);\n#%d=IFCRECTANGLEPROFILEDEF(.AREA.,$,#%d,3.,0.3);\n", id+3, id+4, id+3, id+5, id+4)
		fmt.Fprintf(&b, "#%d=IFCCARTESIANPOINT((0.,0.,0.));\n#%d=IFCAXIS2PLACEMENT3D(#%d,$,$);\n#%d=IFCDIRECTION((0.,0.,1.));\n", id+6, id+7, id+6, id+8)
		fmt.Fprintf(&b, "#%d=IFCEXTRUDEDAREASOLID(#%d,#%d,#%d,2.5);\n#%d=IFCSHAPEREPRESENTATION($,'Body','SweptSolid',(#%d));\n#%d=IFCPRODUCTDEFINITIONSHAPE($,$,(#%d));\n", id+9, id+5, id+7, id+8, id+10, id+9, id+11, id+10)
		fmt.Fprintf(&b, "#%d=IFCWALL('0W%020d',$,'W-%d',$,$,#%d,#%d,$,$);\n", id+12, n, n, id+2, id+11)
		fmt.Fprintf(&b, "#%d=IFCPROPERTYSINGLEVALUE('IsExternal',$,IFCBOOLEAN(.T.),$);\n#%d=IFCPROPERTYSINGLEVALUE('Reference',$,IFCLABEL('W-%d'),$);\n", id+13, id+14, n)
		fmt.Fprintf(&b, "#%d=IFCPROPERTYSET('0P%020d',$,'Pset_WallCommon',$,(#%d,#%d));\n#%d=IFCRELDEFINESBYPROPERTIES('0R%020d',$,$,$,(#%d),#%d);\n", id+15, n, id+13, id+14, id+16, n, id+12, id+15)
		walls = append(walls, id+12)
		id += 20
	}
	fmt.Fprintf(&b, "#%d=IFCRELCONTAINEDINSPATIALSTRUCTURE('0GUIDrc0000000000000009',$,$,$,(", id)
	for i, w := range walls {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "#%d", w)
	}
	b.WriteString("),#6);\nENDSEC;\nEND-ISO-10303-21;\n")
	return b.Bytes()
}

// TestRead_BigFileFitsInMemory reads a synthetic IFC just under the 48 MiB
// cap inside wasm and logs the memory it took. It runs only with
// GOIFC_WASM_BIG=1, in the nightly job.
func TestRead_BigFileFitsInMemory(t *testing.T) {
	if os.Getenv("GOIFC_WASM_BIG") != "1" {
		t.Skip("set GOIFC_WASM_BIG=1 to read a file at the cap")
	}
	src := bigIFC(maxReadBytes)
	if len(src) > maxReadBytes {
		t.Fatalf("the generated file is %d bytes, over the cap", len(src))
	}
	answer, glb := Read(testVersion, src, nil)
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	t.Logf("file %d bytes, answer %d bytes, GLB %d bytes, memory from the system %d MiB", len(src), len(answer), len(glb), m.Sys>>20)
	if !bytes.Contains(answer, []byte(`"ok":true`)) {
		t.Fatalf("answer %.300s, want ok", answer)
	}
	if m.Sys > 2<<30 {
		t.Errorf("memory from the system %d MiB, over 2 GiB", m.Sys>>20)
	}
}
