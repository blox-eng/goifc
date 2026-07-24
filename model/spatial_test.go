package model

import "testing"

// TestSpatialNodes_FullChain: the import contract (import_emit.py) emits the
// spatial containers Site/Building/Storey/Space as nodes — but NOT IfcProject
// (_SPATIAL = Site,Building,Storey,Space) — so the object tree nests elements
// under their storey. model.Extract omits these (physical-only, for the #2211
// parity oracle), so SpatialNodes recovers them for the #2213 cutover.
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
