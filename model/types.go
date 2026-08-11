package model

// Mat4 is a 4x4 transform, column-major (m[col*4+row]); translation at
// 12,13,14 — matches glTF node.matrix LAYOUT. Coordinates remain IFC-native
// (right-handed, Z-up); consumers targeting a Y-up viewer (three.js/r3f) must
// apply a Z-up→Y-up rotation at the scene root.
type Mat4 [16]float64

// Identity returns the 4x4 identity matrix.
func Identity() Mat4 {
	return Mat4{
		1, 0, 0, 0,
		0, 1, 0, 0,
		0, 0, 1, 0,
		0, 0, 0, 1,
	}
}

// Mul returns a*b (a applied to b's result: world = parent.Mul(child)).
func (a Mat4) Mul(b Mat4) Mat4 {
	var out Mat4
	for col := 0; col < 4; col++ {
		for row := 0; row < 4; row++ {
			var s float64
			for k := 0; k < 4; k++ {
				s += a[k*4+row] * b[col*4+k]
			}
			out[col*4+row] = s
		}
	}
	return out
}

// Translation returns the matrix translation component.
func (m Mat4) Translation() (x, y, z float64) { return m[12], m[13], m[14] }

// Quantities holds tier-1 (Qto) scalar quantities in meters/m2/m3. A nil pointer
// means the quantity is absent — never fabricated as 0.0.
type Quantities struct {
	Area, Volume, Length, Width, Height, Perimeter *float64
}

// Element is one semantic IFC element; proxy geometry is built separately by
// the geometry package.
type Element struct {
	GlobalID string
	// ExpressID is the STEP instance id (#id); the geometry package looks the
	// instance back up via step.File.ByID.
	ExpressID      int
	IFCClass       string
	Name           string
	PredefinedType string
	Category       string
	Storey         string
	Material       string
	IsExternal     *bool
	Psets          map[string]map[string]any
	// Qto holds tier-1 Qto scalars (L/W/H frequently nil — only Area gates
	// hasQto/QuantitySource=="qto"). The geometry package's OBB fallback should
	// derive dimensions from the geometry bbox, not rely on these being populated.
	Qto            Quantities
	QuantitySource string // "qto" | "none" | "geometry"
	// Placement is the ObjectPlacement WORLD transform ONLY, with its
	// translation ALREADY scaled to meters (see Result.UnitScale). It does NOT
	// include an IfcMappedItem's mapping transform or a representation item's
	// local Position — the geometry package must compose those itself from the
	// representation (fetched via ExpressID -> step.File.ByID), and must NOT
	// re-scale this translation.
	Placement   Mat4
	ParentIndex *int
	Emit        bool
}

// Result is the output of Extract.
type Result struct {
	Elements []Element
	// UnitScale converts RAW file-unit length values to meters. Element.Placement
	// translations are already scaled by this; representation-item coordinates
	// fetched via Element.ExpressID are NOT — the geometry package must scale
	// those itself.
	UnitScale float64
	Warnings  []string
}
