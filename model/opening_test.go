package model

import "testing"

// TestOpeningsOfTwoVoids ports the inverse of TestStoreyViaFilledVoid's
// voided-element walk: a wall with two IfcRelVoidsElement rels returns both
// IfcOpeningElements.
func TestOpeningsOfTwoVoids(t *testing.T) {
	f := parseString(t, mustRead(t, "testdata/synthetic/wall_two_openings.ifc"))
	wall := f.ByType("IfcWall")[0]
	openings := OpeningsOf(f, wall)
	if len(openings) != 2 {
		t.Fatalf("openings = %v, want 2", openings)
	}
	names := map[string]bool{}
	for _, o := range openings {
		names[strVal(o, attrName)] = true
	}
	if !names["Opening1"] || !names["Opening2"] {
		t.Fatalf("openings = %v, want Opening1 and Opening2", names)
	}
}

func TestOpeningsOfNoVoids(t *testing.T) {
	f := parseString(t, mustRead(t, "testdata/synthetic/wall_no_openings.ifc"))
	wall := f.ByType("IfcWall")[0]
	if openings := OpeningsOf(f, wall); len(openings) != 0 {
		t.Fatalf("openings = %v, want empty", openings)
	}
}

// TestOrphanFillOpenings_Detected: an opening filled by a door but voiding no
// host is reported as an orphan.
func TestOrphanFillOpenings_Detected(t *testing.T) {
	const spf = `ISO-10303-21;
HEADER;
FILE_SCHEMA(('IFC4'));
ENDSEC;
DATA;
#20=IFCOPENINGELEMENT('0GUIDopening00000000020',$,'Orphan',$,$,$,$,$,$);
#25=IFCDOOR('0GUIDdoor00000000000025',$,'D',$,$,$,$,$,$);
#50=IFCRELFILLSELEMENT('0GUIDrel00000000000050',$,$,$,#20,#25);
ENDSEC;
END-ISO-10303-21;`
	f := parseString(t, spf)
	orphans := OrphanFillOpenings(f)
	if len(orphans) != 1 {
		t.Fatalf("orphans = %d, want 1", len(orphans))
	}
	if got := strVal(orphans[0], attrName); got != "Orphan" {
		t.Fatalf("orphan name = %q, want Orphan", got)
	}
}

// TestOrphanFillOpenings_VoidedNotOrphan: an opening that IS voided (fills AND
// voids a host — the normal door-in-wall case) is not an orphan.
func TestOrphanFillOpenings_VoidedNotOrphan(t *testing.T) {
	f := parseString(t, mustRead(t, "testdata/synthetic/door_filled_void.ifc"))
	if orphans := OrphanFillOpenings(f); len(orphans) != 0 {
		t.Fatalf("orphans = %v, want none (opening voids a wall)", orphans)
	}
}

// TestOpeningsOfDanglingRef ensures a rel whose RelatedOpeningElement points
// at a missing instance is skipped rather than panicking or yielding a nil
// *step.Instance entry.
func TestOpeningsOfDanglingRef(t *testing.T) {
	f := parseString(t, mustRead(t, "testdata/synthetic/wall_dangling_opening.ifc"))
	wall := f.ByType("IfcWall")[0]
	if openings := OpeningsOf(f, wall); len(openings) != 0 {
		t.Fatalf("openings = %v, want empty (dangling ref skipped)", openings)
	}
}
