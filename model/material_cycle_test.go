package model

import (
	"fmt"
	"strings"
	"testing"
)

// materialSPF wraps data lines in a minimal IFC4 file with one wall (#50)
// associated to #rel's RelatingMaterial.
func materialSPF(relating string, data ...string) string {
	return "ISO-10303-21;\nHEADER;\nFILE_SCHEMA(('IFC4'));\nENDSEC;\nDATA;\n" +
		strings.Join(data, "\n") + "\n" +
		"#50=IFCWALL('0GUIDwall0000000000050',$,'W',$,$,$,$,$,$);\n" +
		"#61=IFCRELASSOCIATESMATERIAL('0GUIDrel00000000000061',$,$,$,(#50)," + relating + ");\n" +
		"ENDSEC;\nEND-ISO-10303-21;\n"
}

func materialNames(t *testing.T, spf string) []string {
	t.Helper()
	f := parseString(t, spf)
	var names []string
	for _, m := range Materials(f, f.ByType("IfcWall")[0]) {
		names = append(names, strVal(m, 0))
	}
	return names
}

// TestMaterialCycleTerminates covers #104: a material reference cycle used to
// recurse in materialLeaves until the goroutine stack overflowed, a fatal
// error recover() cannot catch. Each shape that can reference its parent must
// terminate and keep only the non-cyclic leaves.
func TestMaterialCycleTerminates(t *testing.T) {
	cases := []struct {
		name     string
		relating string
		data     []string
		want     []string
	}{
		{
			name:     "fuzz crasher: layer set lists its own usage",
			relating: "#0",
			data: []string{
				"#30=IFCMATERIALLAYERSET((#0),'');",
				"#0=IFCMATERIALLAYERSETUSAGE(#30,.AXIS2.,.POSITIVE.,0.,$);",
			},
			want: nil,
		},
		{
			name:     "layer set: usage cycle beside a real layer",
			relating: "#4",
			data: []string{
				"#1=IFCMATERIAL('Brick');",
				"#2=IFCMATERIALLAYER(#1,0.1,$,$,$,$,$);",
				"#3=IFCMATERIALLAYERSET((#2,#4),$,$);",
				"#4=IFCMATERIALLAYERSETUSAGE(#3,.AXIS2.,.POSITIVE.,0.,$);",
			},
			want: []string{"Brick"},
		},
		{
			name:     "layer: material points back at its set",
			relating: "#3",
			data: []string{
				"#1=IFCMATERIAL('Brick');",
				"#2=IFCMATERIALLAYER(#1,0.1,$,$,$,$,$);",
				"#4=IFCMATERIALLAYER(#3,0.1,$,$,$,$,$);",
				"#3=IFCMATERIALLAYERSET((#2,#4),$,$);",
			},
			want: []string{"Brick"},
		},
		{
			name:     "material list contains itself",
			relating: "#2",
			data: []string{
				"#1=IFCMATERIAL('Concrete');",
				"#2=IFCMATERIALLIST((#1,#2));",
			},
			want: []string{"Concrete"},
		},
		{
			name:     "profile set lists its own usage",
			relating: "#4",
			data: []string{
				"#1=IFCMATERIAL('Steel S355');",
				"#2=IFCMATERIALPROFILE($,$,#1,$,$,$);",
				"#3=IFCMATERIALPROFILESET($,$,(#2,#4),$);",
				"#4=IFCMATERIALPROFILESETUSAGE(#3,$,$);",
			},
			want: []string{"Steel S355"},
		},
		{
			name:     "profile: material points back at its set",
			relating: "#3",
			data: []string{
				"#1=IFCMATERIAL('Steel S355');",
				"#2=IFCMATERIALPROFILE($,$,#1,$,$,$);",
				"#4=IFCMATERIALPROFILE($,$,#3,$,$,$);",
				"#3=IFCMATERIALPROFILESET($,$,(#2,#4),$);",
			},
			want: []string{"Steel S355"},
		},
		{
			name:     "constituent: material points back at its set",
			relating: "#5",
			data: []string{
				"#1=IFCMATERIAL('Glass');",
				"#3=IFCMATERIALCONSTITUENT($,$,#1,$,$);",
				"#4=IFCMATERIALCONSTITUENT($,$,#5,$,$);",
				"#5=IFCMATERIALCONSTITUENTSET($,$,(#3,#4));",
			},
			want: []string{"Glass"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := materialNames(t, materialSPF(tc.relating, tc.data...))
			if fmt.Sprint(got) != fmt.Sprint(tc.want) {
				t.Fatalf("materials = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestMaterialSharedLeafKeptPerReference pins the pre-#104 output: a leaf
// reached through two layers is listed once per layer, so the cycle guard
// must not dedupe IfcMaterial leaves.
func TestMaterialSharedLeafKeptPerReference(t *testing.T) {
	got := materialNames(t, materialSPF("#5",
		"#1=IFCMATERIAL('Brick');",
		"#9=IFCMATERIAL('EPS');",
		"#2=IFCMATERIALLAYER(#1,0.1,$,$,$,$,$);",
		"#3=IFCMATERIALLAYER(#9,0.1,$,$,$,$,$);",
		"#4=IFCMATERIALLAYER(#1,0.1,$,$,$,$,$);",
		"#5=IFCMATERIALLAYERSET((#2,#3,#4),$,$);",
	))
	if want := "[Brick EPS Brick]"; fmt.Sprint(got) != want {
		t.Fatalf("materials = %q, want %s", got, want)
	}
}

// listChain builds n nested IfcMaterialLists (#1000 outermost), each holding
// the next `fanout` times, ending in IfcMaterial #1.
func listChain(n, fanout int) []string {
	data := []string{"#1=IFCMATERIAL('Concrete');"}
	for i := 0; i < n; i++ {
		next := "#1"
		if i < n-1 {
			next = fmt.Sprintf("#%d", 1001+i)
		}
		data = append(data, fmt.Sprintf("#%d=IFCMATERIALLIST((%s));", 1000+i,
			strings.TrimSuffix(strings.Repeat(next+",", fanout), ",")))
	}
	return data
}

// TestMaterialDepthBound proves maxMaterialDepth: an acyclic chain whose leaf
// sits exactly at the bound resolves, one level deeper does not.
func TestMaterialDepthBound(t *testing.T) {
	if got := materialNames(t, materialSPF("#1000", listChain(maxMaterialDepth, 1)...)); fmt.Sprint(got) != "[Concrete]" {
		t.Fatalf("chain at the bound: materials = %q, want [Concrete]", got)
	}
	if got := materialNames(t, materialSPF("#1000", listChain(maxMaterialDepth+1, 1)...)); got != nil {
		t.Fatalf("chain past the bound: materials = %q, want none", got)
	}
}

// TestMaterialRepeatedLayerInstanceKept pins the Revit export shape: one
// IfcMaterialLayer instance listed on both faces of a wall is walked each
// time, so the cycle guard must only cut references back to an ancestor.
func TestMaterialRepeatedLayerInstanceKept(t *testing.T) {
	got := materialNames(t, materialSPF("#5",
		"#1=IFCMATERIAL('Plasterboard');",
		"#9=IFCMATERIAL('Stud');",
		"#2=IFCMATERIALLAYER(#1,0.0125,$,$,$,$,$);",
		"#3=IFCMATERIALLAYER(#9,0.092,$,$,$,$,$);",
		"#5=IFCMATERIALLAYERSET((#2,#3,#2),$,$);",
	))
	if want := "[Plasterboard Stud Plasterboard]"; fmt.Sprint(got) != want {
		t.Fatalf("materials = %q, want %s", got, want)
	}
}

// TestMaterialVisitBudget guards the exponential case: each list holds the
// next one twice, so walking every path would visit 2^40 nodes. The walk
// stops at maxMaterialVisits and keeps what it collected.
func TestMaterialVisitBudget(t *testing.T) {
	got := materialNames(t, materialSPF("#1000", listChain(40, 2)...))
	if len(got) == 0 || len(got) > maxMaterialVisits {
		t.Fatalf("got %d materials, want 1..%d", len(got), maxMaterialVisits)
	}
}
