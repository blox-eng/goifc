package model

import (
	"math"
	"testing"
)

// TestSpatialNodes_FullChain: the import contract emits the
// spatial containers Site/Building/Storey/Space as nodes — but NOT IfcProject
// (_SPATIAL = Site,Building,Storey,Space) — so the object tree nests elements
// under their storey. model.Extract omits these (physical-only, for the
// parity oracle), so SpatialNodes recovers them.
func TestSpatialNodes_FullChain(t *testing.T) {
	f := parseString(t, mustRead(t, "testdata/synthetic/spatial_full.ifc"))
	nodes := SpatialNodes(f)

	// site, building, storey, space — NOT IfcProject.
	if len(nodes) != 4 {
		t.Fatalf("want 4 spatial nodes (site,building,storey,space), got %d: %+v", len(nodes), nodes)
	}
	byName := map[string]Element{}
	for _, n := range nodes {
		if n.GlobalID == "" {
			t.Errorf("spatial node missing GlobalID: %+v", n)
		}
		if n.IFCClass == "IFCPROJECT" {
			t.Errorf("IfcProject must NOT be a spatial import node")
		}
		byName[n.Name] = n
	}
	if byName["Site A"].IFCClass != "IFCSITE" {
		t.Errorf("Site A class = %q want IFCSITE", byName["Site A"].IFCClass)
	}
	if byName["Building A"].IFCClass != "IFCBUILDING" {
		t.Errorf("Building A class = %q want IFCBUILDING", byName["Building A"].IFCClass)
	}
	if byName["Level 1"].IFCClass != "IFCBUILDINGSTOREY" {
		t.Errorf("Level 1 class = %q want IFCBUILDINGSTOREY", byName["Level 1"].IFCClass)
	}
	// C3: authored Qto flows through SpatialNodes (IfcSpace GrossArea = 25.0).
	if space := byName["Room 101"]; space.Qto.Area == nil || *space.Qto.Area != 25.0 {
		t.Errorf("Room 101 Qto.Area = %v want 25.0 (spatial Qto)", space.Qto.Area)
	}
}

func TestStoreyElevationsMeters(t *testing.T) {
	const spf = `ISO-10303-21;
HEADER;
FILE_SCHEMA(('IFC4'));
ENDSEC;
DATA;
#1=IFCSIUNIT(*,.LENGTHUNIT.,.MILLI.,.METRE.);
#2=IFCUNITASSIGNMENT((#1));
#10=IFCBUILDINGSTOREY('2Storey0000000000000A',$,'Level 0',$,$,$,$,$,.ELEMENT.,0.);
#11=IFCBUILDINGSTOREY('2Storey0000000000000B',$,'Level 1',$,$,$,$,$,.ELEMENT.,3000.);
ENDSEC;
END-ISO-10303-21;`
	elevs := StoreyElevations(parseString(t, spf))
	if len(elevs) != 2 {
		t.Fatalf("want 2 storeys, got %d: %+v", len(elevs), elevs)
	}
	if got := elevs["2Storey0000000000000A"]; math.Abs(got-0.0) > 1e-9 {
		t.Fatalf("storey A: want 0.0 m, got %v", got)
	}
	if got := elevs["2Storey0000000000000B"]; math.Abs(got-3.0) > 1e-9 { // 3000 mm × 0.001
		t.Fatalf("storey B: want 3.0 m, got %v", got)
	}
}
