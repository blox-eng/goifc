package model

import (
	"testing"

	"github.com/blox-eng/common/ifc/step"
)

// TestTypeIdentity reads the type a typed element points at. The fixture
// type_inheritance.ifc already exists for GetType's own tests: wall #10
// resolves via #41=IFCRELDEFINESBYTYPE(...,(#10,#11),#40) to type #40 =
// IFCWALLTYPE('0GUIDtype0000000000040',$,'WallType',...). Asserting exact
// values (not just non-emptiness) catches a regression where TypeIdentity
// returns the OCCURRENCE's own identity (GlobalId '0GUIDwall0000000000010',
// Name 'W-BARE', class IFCWALL) instead of the TYPE's — non-emptiness alone
// would not detect that.
func TestTypeIdentity(t *testing.T) {
	f, err := step.ParseFile("testdata/synthetic/type_inheritance.ifc")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	walls := f.ByType("IfcWall")
	if len(walls) == 0 {
		t.Fatal("fixture has no wall occurrence")
	}

	gid, name, class := TypeIdentity(f, walls[0])
	if gid != "0GUIDtype0000000000040" {
		t.Errorf("gid = %q, want %q", gid, "0GUIDtype0000000000040")
	}
	if name != "WallType" {
		t.Errorf("name = %q, want %q", name, "WallType")
	}
	if class != "IFCWALLTYPE" {
		t.Errorf("class = %q, want %q", class, "IFCWALLTYPE")
	}
}

// TestTypeIdentityUntyped pins the null case: an element with no
// IfcRelDefinesByType yields three empty strings, never a partial identity.
// Downstream, empty GlobalId is what keeps object_type_id null.
func TestTypeIdentityUntyped(t *testing.T) {
	f, err := step.ParseFile("testdata/synthetic/wall_no_openings.ifc")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	walls := f.ByType("IfcWall")
	if len(walls) == 0 {
		t.Fatal("fixture has no wall occurrence")
	}

	gid, name, class := TypeIdentity(f, walls[0])
	if gid != "" || name != "" || class != "" {
		t.Fatalf("TypeIdentity = (%q, %q, %q), want all empty for an untyped wall", gid, name, class)
	}
}
