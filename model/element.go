package model

import "github.com/blox-eng/goifc/step"

// Psets ports ifcopenshell.util.element.get_psets(should_inherit=True): property
// sets attached to inst via IfcRelDefinesByProperties, merged with property sets
// inherited from inst's type (IfcRelDefinesByType/IsTypedBy). Type psets are
// seeded first; occurrence psets are applied second and override on name
// collision, matching ifcopenshell's inheritance order. When qtosOnly, only
// IfcElementQuantity sets are returned; otherwise only IfcPropertySet. Keys are
// set names; values are {property name -> Go scalar}.
func Psets(f *step.File, inst *step.Instance, qtosOnly bool) map[string]map[string]any {
	out := map[string]map[string]any{}
	if typ := GetType(f, inst); typ != nil {
		for name, props := range typePropertySets(typ, qtosOnly) {
			out[name] = props
		}
	}
	for _, rel := range f.Inverse(inst) {
		if !rel.IsA("IfcRelDefinesByProperties") {
			continue
		}
		def, ok := rel.Ref(attrRel5)
		if !ok {
			continue
		}
		switch {
		case qtosOnly && def.IsA("IfcElementQuantity"):
			out[strVal(def, attrName)] = readQuantitySet(def)
		case !qtosOnly && def.IsA("IfcPropertySet"):
			// Merge per-property, mirroring ifcopenshell.util.element.get_psets'
			// psets.setdefault(name, {}).update(props): the occurrence pset wins
			// on a per-key basis, but type-only keys in a same-named set must
			// survive — a whole-set replace here would silently drop them.
			name := strVal(def, attrName)
			if out[name] == nil {
				out[name] = map[string]any{}
			}
			for k, v := range readPropertySet(def) {
				out[name][k] = v
			}
		}
	}
	return out
}

// GetType ports ifcopenshell.util.element.get_type: the IfcTypeObject (e.g.
// IfcWallType) related to inst via IfcRelDefinesByType (RelatedObjects=attr4,
// RelatingType=attr5). Covers both IFC4 IsTypedBy and IFC2X3
// IfcRelDefinesByType — both are the same entity. nil when inst has no type.
func GetType(f *step.File, inst *step.Instance) *step.Instance {
	for _, rel := range f.Inverse(inst) {
		if !rel.IsA("IfcRelDefinesByType") {
			continue
		}
		// RelatedObjects is attr4 for IfcRelDefinesByType (unlike IfcRelAggregates,
		// where it's attr5 — see instInRelatedObjects). Check membership directly.
		v, ok := rel.Get(attrRel4)
		if !ok || v.Kind != step.KindList {
			continue
		}
		member := false
		for _, item := range v.List {
			if item.Kind == step.KindRef && item.Ref != nil && item.Ref.ID() == inst.ID() {
				member = true
				break
			}
		}
		if !member {
			continue
		}
		if t, ok := rel.Ref(attrRel5); ok { // RelatingType
			return t
		}
	}
	return nil
}

// TypeIdentity returns the identity of inst's IfcTypeObject: its GlobalId, its
// Name, and its entity class. All three are empty when inst carries no
// IfcRelDefinesByType — an element with no type, which must leave
// objects.object_type_id null rather than match on anything else.
//
// The class comes from step.Instance.Type() verbatim, so it is UPPERCASE as the
// file spells it (IFCWALLTYPE). Callers compare case-insensitively.
//
// NOTE: do not filter on the class ending in "Type". IFC2X3 spells door and
// window types IfcDoorStyle / IfcWindowStyle, which a suffix check would
// silently drop.
func TypeIdentity(f *step.File, inst *step.Instance) (globalID, name, class string) {
	typ := GetType(f, inst)
	if typ == nil {
		return "", "", ""
	}
	return strVal(typ, attrGlobalID), strVal(typ, attrName), typ.Type()
}

// typePropertySets reads typ's HasPropertySets (a list of IfcPropertySet or
// IfcElementQuantity, keyed directly — not via a Rel) and returns the subset
// matching qtosOnly, keyed by set Name. Mirrors the type-level half of
// ifcopenshell.util.element.get_psets.
func typePropertySets(typ *step.Instance, qtosOnly bool) map[string]map[string]any {
	out := map[string]map[string]any{}
	v, ok := typ.Get(attrHasPropertySets)
	if !ok || v.Kind != step.KindList {
		return out
	}
	for _, item := range v.List {
		if item.Kind != step.KindRef || item.Ref == nil {
			continue
		}
		def := item.Ref
		switch {
		case qtosOnly && def.IsA("IfcElementQuantity"):
			out[strVal(def, attrName)] = readQuantitySet(def)
		case !qtosOnly && def.IsA("IfcPropertySet"):
			// Defensive per-key merge, consistent with the occurrence overlay in
			// Psets — guards against a malformed type with two same-named psets.
			name := strVal(def, attrName)
			if out[name] == nil {
				out[name] = map[string]any{}
			}
			for k, v := range readPropertySet(def) {
				out[name][k] = v
			}
		}
	}
	return out
}

func readPropertySet(def *step.Instance) map[string]any {
	props := map[string]any{}
	v, ok := def.Get(attrHasProperties)
	if !ok || v.Kind != step.KindList {
		return props
	}
	for _, p := range v.List {
		if p.Kind != step.KindRef || p.Ref == nil || !p.Ref.IsA("IfcPropertySingleValue") {
			continue
		}
		name := strVal(p.Ref, attrPropName)
		if nv, ok := p.Ref.Get(attrNominalValue); ok {
			props[name] = nominalGoValue(nv)
		}
	}
	return props
}

func readQuantitySet(def *step.Instance) map[string]any {
	props := map[string]any{}
	v, ok := def.Get(attrQuantities)
	if !ok || v.Kind != step.KindList {
		return props
	}
	for _, q := range v.List {
		if q.Kind != step.KindRef || q.Ref == nil {
			continue
		}
		if val, has := floatAt(q.Ref, attrQuantityValue); has {
			props[strVal(q.Ref, attrPropName)] = val
		}
	}
	return props
}

// Container ports util.element.get_container: the spatial element that contains
// inst via IfcRelContainedInSpatialStructure, or the decomposition parent via
// IfcRelAggregates. Returns nil when uncontained.
//
// This is the DIRECT parent only — used for ParentIndex (the spatial tree).
// It intentionally does NOT recurse through nesting/filled-void/voided-element
// (see containerOf for the full ifcopenshell get_container chain used by Storey).
func Container(f *step.File, inst *step.Instance) *step.Instance {
	for _, rel := range f.Inverse(inst) {
		if rel.IsA("IfcRelContainedInSpatialStructure") {
			if s, ok := rel.Ref(attrRel5); ok { // RelatingStructure
				return s
			}
		}
	}
	if p := getAggregate(f, inst); p != nil {
		return p
	}
	return nil
}

// getAggregate returns inst's IfcRelAggregates parent (inst appears in
// RelatedObjects → RelatingObject is the parent). f.Inverse(inst) returns
// every rel referencing inst by ANY attribute, so for
// IfcRelAggregates(...,RelatingObject=#10,RelatedObjects=(#20)) it also
// returns the rel when inst==#10 (the parent itself, referenced at attr4).
// Without the RelatedObjects membership check, the parent would resolve to
// itself — a self-cycle.
func getAggregate(f *step.File, inst *step.Instance) *step.Instance {
	for _, rel := range f.Inverse(inst) {
		if rel.IsA("IfcRelAggregates") && instInList(rel, attrRel5, inst) {
			if p, ok := rel.Ref(attrRel4); ok { // RelatingObject is at index 4 for IfcRelAggregates
				return p
			}
		}
	}
	return nil
}

// getNest returns inst's IfcRelNests parent: IfcRelNests(...,RelatingObject=4,
// RelatedObjects=5) — same attribute layout as IfcRelAggregates.
func getNest(f *step.File, inst *step.Instance) *step.Instance {
	for _, rel := range f.Inverse(inst) {
		if rel.IsA("IfcRelNests") && instInList(rel, attrRel5, inst) {
			if p, ok := rel.Ref(attrRel4); ok { // RelatingObject
				return p
			}
		}
	}
	return nil
}

// getFilledVoid returns the IfcOpeningElement inst (a building element, e.g.
// a door/window) fills: IfcRelFillsElement(...,RelatingOpeningElement=4,
// RelatedBuildingElement=5), both single refs.
func getFilledVoid(f *step.File, inst *step.Instance) *step.Instance {
	for _, rel := range f.Inverse(inst) {
		if !rel.IsA("IfcRelFillsElement") {
			continue
		}
		if related, ok := rel.Ref(attrRel5); ok && related.ID() == inst.ID() { // RelatedBuildingElement
			if opening, ok := rel.Ref(attrRel4); ok { // RelatingOpeningElement
				return opening
			}
		}
	}
	return nil
}

// getVoidedElement returns the building element (e.g. a wall) inst (an
// IfcOpeningElement) voids: IfcRelVoidsElement(...,RelatingBuildingElement=4,
// RelatedOpeningElement=5), both single refs.
func getVoidedElement(f *step.File, inst *step.Instance) *step.Instance {
	for _, rel := range f.Inverse(inst) {
		if !rel.IsA("IfcRelVoidsElement") {
			continue
		}
		if related, ok := rel.Ref(attrRel5); ok && related.ID() == inst.ID() { // RelatedOpeningElement
			if voided, ok := rel.Ref(attrRel4); ok { // RelatingBuildingElement
				return voided
			}
		}
	}
	return nil
}

// OpeningsOf returns the IfcOpeningElement instances that void host
// (the inverse of getVoidedElement): every IfcRelVoidsElement whose
// RelatingBuildingElement is host, yielding its RelatedOpeningElement.
func OpeningsOf(f *step.File, host *step.Instance) []*step.Instance {
	var out []*step.Instance
	for _, rel := range f.Inverse(host) {
		if !rel.IsA("IfcRelVoidsElement") {
			continue
		}
		if voided, ok := rel.Ref(attrRel4); ok && voided.ID() == host.ID() { // RelatingBuildingElement
			if opening, ok := rel.Ref(attrRel5); ok { // RelatedOpeningElement
				out = append(out, opening)
			}
		}
	}
	return out
}

// OrphanFillOpenings returns every IfcOpeningElement that is FILLED (referenced
// as the RelatingOpeningElement of an IfcRelFillsElement) yet voids NO host
// (no IfcRelVoidsElement names it as RelatedOpeningElement). Such a
// filled-but-void-less opening is a benign IFC inconsistency worth one
// aggregated warning. Keeping this rel-walk in the model package lets the
// geometry engine stay free of IfcRelFills/IfcRelVoids traversal. Each opening
// is returned once; nil when there are none.
func OrphanFillOpenings(f *step.File) []*step.Instance {
	var out []*step.Instance
	seen := map[int]bool{}
	for _, rel := range f.ByType("IfcRelFillsElement") {
		opening, ok := rel.Ref(attrRel4) // RelatingOpeningElement
		if !ok || opening == nil || seen[opening.ID()] {
			continue
		}
		if getVoidedElement(f, opening) == nil { // filled but voids no host
			seen[opening.ID()] = true
			out = append(out, opening)
		}
	}
	return out
}

// getParent ports ifcopenshell.util.element.get_parent: the first of
// aggregate/nest/filled-void/voided-element parent, in that order.
func getParent(f *step.File, inst *step.Instance) *step.Instance {
	if p := getAggregate(f, inst); p != nil {
		return p
	}
	if p := getNest(f, inst); p != nil {
		return p
	}
	if p := getFilledVoid(f, inst); p != nil {
		return p
	}
	if p := getVoidedElement(f, inst); p != nil {
		return p
	}
	return nil
}

// instInList reports whether inst appears (by reference) in rel's list-valued
// attribute at attrIdx — e.g. RelatedObjects (attr5) for IfcRelAggregates/
// IfcRelNests, or RelatedElements (attr4) for
// IfcRelContainedInSpatialStructure.
func instInList(rel *step.Instance, attrIdx int, inst *step.Instance) bool {
	v, ok := rel.Get(attrIdx)
	if !ok || v.Kind != step.KindList {
		return false
	}
	for _, item := range v.List {
		if item.Kind == step.KindRef && item.Ref != nil && item.Ref.ID() == inst.ID() {
			return true
		}
	}
	return false
}

// containerOf ports ifcopenshell.util.element.get_container (recursive): if
// inst is directly contained via IfcRelContainedInSpatialStructure, return
// the RelatingStructure; otherwise recurse on get_parent (aggregate, nest,
// filled-void, voided-element), matching ifcopenshell's chain: e.g. a door
// resolves door→(filled_void)→opening→(voided_element)→wall→(contained)→
// storey. depth guards against malformed/cyclic IFC.
func containerOf(f *step.File, inst *step.Instance, depth int) *step.Instance {
	if depth <= 0 {
		return nil
	}
	for _, rel := range f.Inverse(inst) {
		if rel.IsA("IfcRelContainedInSpatialStructure") && instInList(rel, attrRel4, inst) {
			if s, ok := rel.Ref(attrRel5); ok { // RelatingStructure
				return s
			}
		}
	}
	if p := getParent(f, inst); p != nil {
		return containerOf(f, p, depth-1)
	}
	return nil
}

// Storey ports decompose._storey: resolves inst's enclosing IfcBuildingStorey
// name via containerOf (ifcopenshell get_container, recursing through
// nesting/filled-void/voiding when inst isn't directly contained), climbing
// up to 8 hops.
func Storey(f *step.File, inst *step.Instance) string {
	c := containerOf(f, inst, 32)
	for i := 0; i < 8 && c != nil; i++ {
		if c.IsA("IfcBuildingStorey") {
			return strVal(c, attrName)
		}
		c = containerOf(f, c, 32)
	}
	return ""
}

// Materials ports util.element.get_materials (best-effort): named IfcMaterial(s)
// associated via IfcRelAssociatesMaterial. Direct IfcMaterial and layer set/list
// wrappers are both resolved down to their IfcMaterial leaves. Mirrors
// ifcopenshell.util.element.get_material: falls back to the element's TYPE
// material association when the occurrence itself has none.
func Materials(f *step.File, inst *step.Instance) []*step.Instance {
	out := materialsDirect(f, inst)
	if len(out) > 0 {
		return out
	}
	if typ := GetType(f, inst); typ != nil {
		return materialsDirect(f, typ)
	}
	return out
}

func materialsDirect(f *step.File, inst *step.Instance) []*step.Instance {
	var out []*step.Instance
	for _, rel := range f.Inverse(inst) {
		if !rel.IsA("IfcRelAssociatesMaterial") {
			continue
		}
		m, ok := rel.Ref(attrRel5) // RelatingMaterial
		if !ok {
			continue
		}
		switch {
		case m.IsA("IfcMaterial"):
			out = append(out, m)
		case m.IsA("IfcMaterialLayerSetUsage"), m.IsA("IfcMaterialLayerSet"), m.IsA("IfcMaterialList"),
			m.IsA("IfcMaterialProfileSetUsage"), m.IsA("IfcMaterialProfileSet"),
			m.IsA("IfcMaterialConstituentSet"):
			out = append(out, materialLeaves(f, m)...)
		}
	}
	return out
}

// IfcMaterialLayerSetUsage: [ForLayerSet,LayerSetDirection,DirectionSense,OffsetFromReferenceLine,...]
const attrForLayerSet = 0

// IfcMaterialLayerSet: [MaterialLayers,LayerSetName,Description]
const attrMaterialLayers = 0

// IfcMaterialLayer: [Material,LayerThickness,IsVentilated,Name,Description,Category,Priority]
const attrLayerMaterial = 0

// IfcMaterialList: [Materials]
const attrMaterialListMaterials = 0

// IfcMaterialProfileSetUsage: [ForProfileSet,CardinalPoint,ReferenceExtent]
const attrForProfileSet = 0

// IfcMaterialProfileSet: [Name,Description,MaterialProfiles,CompositeProfile]
const attrMaterialProfiles = 2

// IfcMaterialProfile: [Name,Description,Material,Profile,Priority,Category]
const attrProfileMaterial = 2

// IfcMaterialConstituentSet: [Name,Description,MaterialConstituents]
const attrMaterialConstituents = 2

// IfcMaterialConstituent: [Name,Description,Material,Fraction,Category]
const attrConstituentMaterial = 2

// materialLeaves resolves m (e.g. a layer set usage / layer set / material
// list / profile set / constituent set) down to its IfcMaterial leaves,
// walking the declared EXPRESS LIST order rather than the forward reference
// graph (graph-DFS order does not preserve IfcMaterialLayerSet.MaterialLayers
// order and can return the wrong layer's material first). Mirrors
// ifcopenshell.util.element.get_materials.
func materialLeaves(f *step.File, m *step.Instance) []*step.Instance {
	switch {
	case m.IsA("IfcMaterial"):
		return []*step.Instance{m}

	case m.IsA("IfcMaterialLayerSetUsage"):
		if set, ok := m.Ref(attrForLayerSet); ok {
			return materialLeaves(f, set)
		}
		return nil

	case m.IsA("IfcMaterialLayerSet"):
		v, ok := m.Get(attrMaterialLayers)
		if !ok || v.Kind != step.KindList {
			return nil
		}
		var out []*step.Instance
		for _, item := range v.List {
			if item.Kind != step.KindRef || item.Ref == nil {
				continue
			}
			out = append(out, materialLeaves(f, item.Ref)...)
		}
		return out

	case m.IsA("IfcMaterialLayer"):
		mat, ok := m.Ref(attrLayerMaterial)
		if !ok {
			return nil // Material is optional/nullable
		}
		return materialLeaves(f, mat)

	case m.IsA("IfcMaterialList"):
		v, ok := m.Get(attrMaterialListMaterials)
		if !ok || v.Kind != step.KindList {
			return nil
		}
		var out []*step.Instance
		for _, item := range v.List {
			if item.Kind != step.KindRef || item.Ref == nil {
				continue
			}
			out = append(out, materialLeaves(f, item.Ref)...)
		}
		return out

	case m.IsA("IfcMaterialProfileSetUsage"):
		if set, ok := m.Ref(attrForProfileSet); ok {
			return materialLeaves(f, set)
		}
		return nil

	case m.IsA("IfcMaterialProfileSet"):
		v, ok := m.Get(attrMaterialProfiles)
		if !ok || v.Kind != step.KindList {
			return nil
		}
		var out []*step.Instance
		for _, item := range v.List {
			if item.Kind != step.KindRef || item.Ref == nil {
				continue
			}
			out = append(out, materialLeaves(f, item.Ref)...)
		}
		return out

	case m.IsA("IfcMaterialProfile"):
		mat, ok := m.Ref(attrProfileMaterial)
		if !ok {
			return nil // Material is optional/nullable
		}
		return materialLeaves(f, mat)

	case m.IsA("IfcMaterialConstituentSet"):
		v, ok := m.Get(attrMaterialConstituents)
		if !ok || v.Kind != step.KindList {
			return nil
		}
		var out []*step.Instance
		for _, item := range v.List {
			if item.Kind != step.KindRef || item.Ref == nil {
				continue
			}
			out = append(out, materialLeaves(f, item.Ref)...)
		}
		return out

	case m.IsA("IfcMaterialConstituent"):
		mat, ok := m.Ref(attrConstituentMaterial)
		if !ok {
			return nil // Material is optional/nullable
		}
		return materialLeaves(f, mat)
	}
	return nil
}

// IsExternal returns the tri-state *Common.IsExternal pset value: nil when no
// *Common pset carries a boolean IsExternal property (including the EXPRESS
// LOGICAL .U. "unknown" case, which nominalGoValue already maps to nil).
func IsExternal(f *step.File, inst *step.Instance) *bool {
	for name, props := range Psets(f, inst, false) {
		if len(name) >= 6 && name[len(name)-6:] == "Common" {
			if v, ok := props["IsExternal"]; ok {
				if b, isBool := v.(bool); isBool {
					return &b
				}
			}
		}
	}
	return nil
}

// nominalGoValue unwraps an IfcValue (typically a typed wrapper like
// IFCBOOLEAN(.T.), IFCLABEL('x'), IFCREAL(1.2)) into a Go scalar.
func nominalGoValue(v step.Value) any {
	inner := v
	if v.Kind == step.KindTyped && len(v.List) == 1 {
		inner = v.List[0]
	}
	switch inner.Kind {
	case step.KindBool:
		return inner.B
	case step.KindFloat:
		return inner.F
	case step.KindInt:
		return inner.I
	case step.KindString, step.KindEnum:
		return inner.Str
	case step.KindLogical:
		return nil // .U. unknown
	}
	return nil
}
