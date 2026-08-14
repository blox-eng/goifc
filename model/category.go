package model

import (
	"strings"

	"github.com/blox-eng/goifc/step"
)

// predefinedType returns the PredefinedType enum label (dots already stripped
// by the tokenizer) for the instance's class, or "" when the class has no
// PredefinedType attribute in this schema, the attribute is unset, or it is
// not an enum value.
//
// The PredefinedType attribute's positional index varies by (schema, class) —
// e.g. index 8 for IfcWall/IfcSlab/IfcColumn/IfcBeam/IfcCovering/IfcRoof, but
// index 10 for IfcDoor/IfcWindow (they carry extra OverallHeight/OverallWidth
// attributes) and index 12 for IfcStairFlight. Reading a fixed index for every
// class silently mis-decodes those (returns "" or the wrong attribute's
// value), so the index is looked up per (schema, class) below.
func predefinedType(f *step.File, inst *step.Instance) string {
	idxByClass := predefinedTypeIndexIFC4
	if strings.Contains(f.SchemaID(), "IFC2X3") {
		idxByClass = predefinedTypeIndexIFC2X3
	}
	idx, ok := idxByClass[strings.ToUpper(inst.Type())]
	if !ok {
		return ""
	}
	v, ok := inst.Get(idx)
	if !ok || v.Kind != step.KindEnum {
		return ""
	}
	return v.Str
}

// predefinedTypeIndexIFC4 and predefinedTypeIndexIFC2X3 map UPPER-CASE IFC
// class name → the 0-based positional index (all attributes, inherited +
// own, in EXPRESS declaration order) of that class's PredefinedType
// attribute. Schema-generated from ifcopenshell (all_attributes() per
// IfcElement subtype that declares a "PredefinedType" attribute) — do not
// hand-edit; regenerate from the schema if entries are missing or wrong.
var predefinedTypeIndexIFC4 = map[string]int{
	"IFCACTUATOR":                     8,
	"IFCAIRTERMINAL":                  8,
	"IFCAIRTERMINALBOX":               8,
	"IFCAIRTOAIRHEATRECOVERY":         8,
	"IFCALARM":                        8,
	"IFCAUDIOVISUALAPPLIANCE":         8,
	"IFCBEAM":                         8,
	"IFCBEAMSTANDARDCASE":             8,
	"IFCBOILER":                       8,
	"IFCBUILDINGELEMENTPART":          8,
	"IFCBUILDINGELEMENTPROXY":         8,
	"IFCBURNER":                       8,
	"IFCCABLECARRIERFITTING":          8,
	"IFCCABLECARRIERSEGMENT":          8,
	"IFCCABLEFITTING":                 8,
	"IFCCABLESEGMENT":                 8,
	"IFCCHILLER":                      8,
	"IFCCHIMNEY":                      8,
	"IFCCOIL":                         8,
	"IFCCOLUMN":                       8,
	"IFCCOLUMNSTANDARDCASE":           8,
	"IFCCOMMUNICATIONSAPPLIANCE":      8,
	"IFCCOMPRESSOR":                   8,
	"IFCCONDENSER":                    8,
	"IFCCONTROLLER":                   8,
	"IFCCOOLEDBEAM":                   8,
	"IFCCOOLINGTOWER":                 8,
	"IFCCOVERING":                     8,
	"IFCCURTAINWALL":                  8,
	"IFCDAMPER":                       8,
	"IFCDISCRETEACCESSORY":            8,
	"IFCDISTRIBUTIONCHAMBERELEMENT":   8,
	"IFCDOOR":                         10,
	"IFCDOORSTANDARDCASE":             10,
	"IFCDUCTFITTING":                  8,
	"IFCDUCTSEGMENT":                  8,
	"IFCDUCTSILENCER":                 8,
	"IFCELECTRICAPPLIANCE":            8,
	"IFCELECTRICDISTRIBUTIONBOARD":    8,
	"IFCELECTRICFLOWSTORAGEDEVICE":    8,
	"IFCELECTRICGENERATOR":            8,
	"IFCELECTRICMOTOR":                8,
	"IFCELECTRICTIMECONTROL":          8,
	"IFCELEMENTASSEMBLY":              9,
	"IFCENGINE":                       8,
	"IFCEVAPORATIVECOOLER":            8,
	"IFCEVAPORATOR":                   8,
	"IFCFAN":                          8,
	"IFCFASTENER":                     8,
	"IFCFILTER":                       8,
	"IFCFIRESUPPRESSIONTERMINAL":      8,
	"IFCFLOWINSTRUMENT":               8,
	"IFCFLOWMETER":                    8,
	"IFCFOOTING":                      8,
	"IFCFURNITURE":                    8,
	"IFCGEOGRAPHICELEMENT":            8,
	"IFCHEATEXCHANGER":                8,
	"IFCHUMIDIFIER":                   8,
	"IFCINTERCEPTOR":                  8,
	"IFCJUNCTIONBOX":                  8,
	"IFCLAMP":                         8,
	"IFCLIGHTFIXTURE":                 8,
	"IFCMECHANICALFASTENER":           10,
	"IFCMEDICALDEVICE":                8,
	"IFCMEMBER":                       8,
	"IFCMEMBERSTANDARDCASE":           8,
	"IFCMOTORCONNECTION":              8,
	"IFCOPENINGELEMENT":               8,
	"IFCOPENINGSTANDARDCASE":          8,
	"IFCOUTLET":                       8,
	"IFCPILE":                         8,
	"IFCPIPEFITTING":                  8,
	"IFCPIPESEGMENT":                  8,
	"IFCPLATE":                        8,
	"IFCPLATESTANDARDCASE":            8,
	"IFCPROJECTIONELEMENT":            8,
	"IFCPROTECTIVEDEVICE":             8,
	"IFCPROTECTIVEDEVICETRIPPINGUNIT": 8,
	"IFCPUMP":                         8,
	"IFCRAILING":                      8,
	"IFCRAMP":                         8,
	"IFCRAMPFLIGHT":                   8,
	"IFCREINFORCINGBAR":               12,
	"IFCREINFORCINGMESH":              17,
	"IFCROOF":                         8,
	"IFCSANITARYTERMINAL":             8,
	"IFCSENSOR":                       8,
	"IFCSHADINGDEVICE":                8,
	"IFCSLAB":                         8,
	"IFCSLABELEMENTEDCASE":            8,
	"IFCSLABSTANDARDCASE":             8,
	"IFCSOLARDEVICE":                  8,
	"IFCSPACEHEATER":                  8,
	"IFCSTACKTERMINAL":                8,
	"IFCSTAIR":                        8,
	"IFCSTAIRFLIGHT":                  12,
	"IFCSURFACEFEATURE":               8,
	"IFCSWITCHINGDEVICE":              8,
	"IFCSYSTEMFURNITUREELEMENT":       8,
	"IFCTANK":                         8,
	"IFCTENDON":                       9,
	"IFCTENDONANCHOR":                 9,
	"IFCTRANSFORMER":                  8,
	"IFCTRANSPORTELEMENT":             8,
	"IFCTUBEBUNDLE":                   8,
	"IFCUNITARYCONTROLELEMENT":        8,
	"IFCUNITARYEQUIPMENT":             8,
	"IFCVALVE":                        8,
	"IFCVIBRATIONISOLATOR":            8,
	"IFCVOIDINGFEATURE":               8,
	"IFCWALL":                         8,
	"IFCWALLELEMENTEDCASE":            8,
	"IFCWALLSTANDARDCASE":             8,
	"IFCWASTETERMINAL":                8,
	"IFCWINDOW":                       10,
	"IFCWINDOWSTANDARDCASE":           10,
}

// predefinedTypeIndexIFC2X3 — IFC2X3's IfcElement subtree only added
// PredefinedType to a handful of classes (door/window/stairflight gained it
// later, in IFC4). IfcRamp/IfcRoof/IfcStair carry the equivalent enum under
// the legacy name "ShapeType" at the same position — included here since the
// oracle resolves it via getattr(elem,"PredefinedType") or
// getattr(elem,"ShapeType").
var predefinedTypeIndexIFC2X3 = map[string]int{
	"IFCCOVERING":        8,
	"IFCELEMENTASSEMBLY": 9,
	"IFCFOOTING":         8,
	"IFCPILE":            8,
	"IFCRAILING":         8,
	"IFCRAMP":            8, // ShapeType
	"IFCROOF":            8, // ShapeType
	"IFCSLAB":            8,
	"IFCSTAIR":           8, // ShapeType
	"IFCTENDON":          9,
}

// Category ports decompose.py:_category — a coarse construction category derived
// from the IFC class + PredefinedType (never geometry).
func Category(f *step.File, inst *step.Instance) string {
	cls := strings.ToUpper(inst.Type())
	pdt := strings.ToUpper(predefinedType(f, inst))
	switch {
	case cls == "IFCROOF":
		return "ROOF"
	case strings.HasPrefix(cls, "IFCSLAB"):
		switch pdt {
		case "ROOF":
			return "ROOF"
		case "LANDING":
			return "LANDING"
		case "BASESLAB":
			return "BASESLAB"
		default:
			return "FLOOR"
		}
	case cls == "IFCCOVERING":
		switch pdt {
		case "ROOFING":
			return "ROOF"
		case "CLADDING":
			return "CLADDING"
		case "CEILING":
			return "CEILING"
		case "FLOORING":
			return "FLOORING"
		default:
			return "COVERING"
		}
	case strings.HasPrefix(cls, "IFCWALL"):
		return "WALL"
	case strings.HasPrefix(cls, "IFCCOLUMN"):
		return "COLUMN"
	case strings.HasPrefix(cls, "IFCBEAM"):
		return "BEAM"
	}
	return inst.Type() // original-case class as fallback (matches is_a())
}

var structuralPrefixes = []string{
	"IFCWALL", "IFCSLAB", "IFCCOLUMN", "IFCBEAM", "IFCROOF", "IFCSTAIR", "IFCRAMP",
}

// isStructural reports whether class becomes a priced base object. Spatial
// containers are handled separately in Extract.
func isStructural(class string) bool {
	c := strings.ToUpper(class)
	for _, p := range structuralPrefixes {
		if strings.HasPrefix(c, p) {
			return true
		}
	}
	return false
}
