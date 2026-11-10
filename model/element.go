package model

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/blox-eng/goifc/step"
)

// Psets ports ifcopenshell.util.element.get_psets(should_inherit=True): property
// sets attached to inst via IfcRelDefinesByProperties, merged with property sets
// inherited from inst's type (IfcRelDefinesByType/IsTypedBy). Type psets are
// seeded first; occurrence psets are applied second and override on name
// collision, matching ifcopenshell's inheritance order. When qtosOnly, only
// IfcElementQuantity sets are returned; otherwise only IfcPropertySet. Keys are
// set names; values are {property name -> Go scalar}.
func Psets(f *step.File, inst *step.Instance, qtosOnly bool) map[string]map[string]any {
	return newPropertySets(f).of(inst, qtosOnly)
}

// propertySets reads property and quantity sets for one pass over a file. It
// reads each set once, and elements that name the same sets share one result:
// a set shared by K elements is not copied K times. A result it returns may be
// shared, so callers must not change it.
type propertySets struct {
	f     *step.File
	sets  map[int]map[string]any                // IfcPropertySet or IfcElementQuantity -> its values
	types map[setsKey]map[string]map[string]any // a type's own sets
	byDef map[string]map[string]map[string]any  // type and occurrence sets -> the merged result
	qtos  map[int]map[string]float64            // a type's quantities, flattened

	// left is how many more set entries r may copy into results of its own;
	// below zero is unlimited. dropped are the elements whose sets did not fit.
	left    int
	dropped map[int]bool
}

type setsKey struct {
	id       int
	qtosOnly bool
}

func newPropertySets(f *step.File) *propertySets {
	return &propertySets{
		f:     f,
		sets:  map[int]map[string]any{},
		types: map[setsKey]map[string]map[string]any{},
		byDef: map[string]map[string]map[string]any{},
		qtos:  map[int]map[string]float64{},
		left:  -1,
	}
}

// setEntriesPerInstance sizes [Extract]'s set budget: the entries it may copy,
// per instance in the file. A set shared as a whole costs nothing, so the
// budget only meets elements that each carry a set of their own beside a type
// with many sets, where every element needs its own copy of the type's sets.
// The corpus copies at most 0.13 entries per instance (office_a), so 16 leaves
// over a hundredfold headroom.
const setEntriesPerInstance = 16

// limitCopies makes r leave out an element's sets once copying them would pass
// setEntriesPerInstance entries per instance in the file.
func (r *propertySets) limitCopies() {
	r.left = setEntriesPerInstance*r.f.Len() + 1<<16
	r.dropped = map[int]bool{}
}

// read returns def's values, read once.
func (r *propertySets) read(def *step.Instance) map[string]any {
	if m, ok := r.sets[def.ID()]; ok {
		return m
	}
	var m map[string]any
	if def.IsA("IfcElementQuantity") {
		m = readQuantitySet(def)
	} else {
		m = readPropertySet(def)
	}
	r.sets[def.ID()] = m
	return m
}

// occurrenceSets returns the sets attached to inst by IfcRelDefinesByProperties
// that match qtosOnly, in relation order.
func (r *propertySets) occurrenceSets(inst *step.Instance, qtosOnly bool) []*step.Instance {
	var defs []*step.Instance
	for _, rel := range r.f.Inverse(inst) {
		if !rel.IsA("IfcRelDefinesByProperties") {
			continue
		}
		def, ok := rel.Ref(attrRel5)
		if !ok {
			continue
		}
		if (qtosOnly && def.IsA("IfcElementQuantity")) || (!qtosOnly && def.IsA("IfcPropertySet")) {
			defs = append(defs, def)
		}
	}
	return defs
}

func (r *propertySets) of(inst *step.Instance, qtosOnly bool) map[string]map[string]any {
	typ := GetType(r.f, inst)
	defs := r.occurrenceSets(inst, qtosOnly)
	var key strings.Builder
	fmt.Fprintf(&key, "%t", qtosOnly)
	if typ != nil {
		fmt.Fprintf(&key, " t%d", typ.ID())
	}
	for _, d := range defs {
		fmt.Fprintf(&key, " %d", d.ID())
	}
	if out, ok := r.byDef[key.String()]; ok {
		if out == nil {
			r.dropped[inst.ID()] = true
		}
		return out
	}
	var inherited map[string]map[string]any
	if typ != nil {
		inherited = r.typeSets(typ, qtosOnly)
	}
	if len(defs) == 0 && inherited != nil {
		r.byDef[key.String()] = inherited // nothing to merge: share the type's sets whole
		return inherited
	}
	drop := func() map[string]map[string]any {
		r.byDef[key.String()] = nil
		r.dropped[inst.ID()] = true
		return nil
	}
	if !r.charge(len(inherited) + len(defs)) {
		return drop()
	}
	out := make(map[string]map[string]any, len(inherited)+len(defs))
	maps.Copy(out, inherited)
	owned := map[string]bool{} // sets this result copied, so it may change them
	for _, def := range defs {
		name := strVal(def, attrName)
		if qtosOnly {
			out[name] = r.read(def)
			continue
		}
		// Merge per-property, mirroring ifcopenshell.util.element.get_psets'
		// psets.setdefault(name, {}).update(props): the occurrence pset wins
		// on a per-key basis, but type-only keys in a same-named set must
		// survive — a whole-set replace here would silently drop them.
		cur, seen := out[name]
		if !seen {
			out[name] = r.read(def)
			continue
		}
		// A merge copies the set it merges into, once, and then each set merged
		// in: charge both before copying them.
		add := r.read(def)
		cost := len(add)
		if !owned[name] {
			cost += len(cur)
		}
		if !r.charge(cost) {
			return drop()
		}
		if !owned[name] {
			cur = maps.Clone(cur)
			owned[name] = true
		}
		maps.Copy(cur, add)
		out[name] = cur
	}
	r.byDef[key.String()] = out
	return out
}

// charge takes n entries from r's budget, and reports whether they fit. An
// unlimited r always has room.
func (r *propertySets) charge(n int) bool {
	if r.left < 0 {
		return true
	}
	if n > r.left {
		return false
	}
	r.left -= n
	return true
}

// typeSets returns typ's own sets that match qtosOnly, read once per type.
func (r *propertySets) typeSets(typ *step.Instance, qtosOnly bool) map[string]map[string]any {
	k := setsKey{typ.ID(), qtosOnly}
	if out, ok := r.types[k]; ok {
		return out
	}
	out := typePropertySets(typ, qtosOnly, r.read)
	r.types[k] = out
	return out
}

// typeQuantities returns typ's quantities as one flat map, read once per
// type. Sets are flattened in name order, so a quantity two sets both name
// takes the same value on every run.
func (r *propertySets) typeQuantities(typ *step.Instance) map[string]float64 {
	if flat, ok := r.qtos[typ.ID()]; ok {
		return flat
	}
	sets := r.typeSets(typ, true)
	flat := map[string]float64{}
	for _, name := range slices.Sorted(maps.Keys(sets)) {
		for q, v := range sets[name] {
			if val, ok := v.(float64); ok {
				flat[q] = val
			}
		}
	}
	r.qtos[typ.ID()] = flat
	return flat
}

// isExternal is [IsExternal] over inst's sets as r reads them.
func (r *propertySets) isExternal(inst *step.Instance) *bool {
	return isExternalIn(r.of(inst, false))
}

// GetType ports ifcopenshell.util.element.get_type: the IfcTypeObject (e.g.
// IfcWallType) related to inst via IfcRelDefinesByType (RelatedObjects=attr4,
// RelatingType=attr5). Covers both IFC4 IsTypedBy and IFC2X3
// IfcRelDefinesByType — both are the same entity. nil when inst has no type.
func GetType(f *step.File, inst *step.Instance) *step.Instance {
	// RelatedObjects is attr4 for IfcRelDefinesByType (unlike IfcRelAggregates,
	// where it's attr5).
	return memberOf(f, inst, "IfcRelDefinesByType", attrRel4, attrRel5) // RelatingType
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
func typePropertySets(typ *step.Instance, qtosOnly bool, read func(*step.Instance) map[string]any) map[string]map[string]any {
	out := map[string]map[string]any{}
	v, ok := typ.Get(attrHasPropertySets)
	if !ok || v.Kind() != step.KindList {
		return out
	}
	owned := map[string]bool{}
	for _, item := range v.List() {
		if item.Kind() != step.KindRef || item.Ref() == nil {
			continue
		}
		def := item.Ref()
		switch {
		case qtosOnly && def.IsA("IfcElementQuantity"):
			out[strVal(def, attrName)] = read(def)
		case !qtosOnly && def.IsA("IfcPropertySet"):
			// Defensive per-key merge, consistent with the occurrence overlay in
			// Psets — guards against a malformed type with two same-named psets.
			name := strVal(def, attrName)
			cur, seen := out[name]
			if !seen {
				out[name] = read(def)
				continue
			}
			if !owned[name] {
				cur = maps.Clone(cur)
				owned[name] = true
			}
			maps.Copy(cur, read(def))
			out[name] = cur
		}
	}
	return out
}

func readPropertySet(def *step.Instance) map[string]any {
	props := map[string]any{}
	v, ok := def.Get(attrHasProperties)
	if !ok || v.Kind() != step.KindList {
		return props
	}
	for _, p := range v.List() {
		if p.Kind() != step.KindRef || p.Ref() == nil || !p.Ref().IsA("IfcPropertySingleValue") {
			continue
		}
		name := strVal(p.Ref(), attrPropName)
		if nv, ok := p.Ref().Get(attrNominalValue); ok {
			props[name] = nominalGoValue(nv)
		}
	}
	return props
}

func readQuantitySet(def *step.Instance) map[string]any {
	props := map[string]any{}
	v, ok := def.Get(attrQuantities)
	if !ok || v.Kind() != step.KindList {
		return props
	}
	for _, q := range v.List() {
		if q.Kind() != step.KindRef || q.Ref() == nil {
			continue
		}
		if val, has := floatAt(q.Ref(), attrQuantityValue); has {
			props[strVal(q.Ref(), attrPropName)] = val
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
	return memberOf(f, inst, "IfcRelAggregates", attrRel5, attrRel4) // RelatingObject
}

// getNest returns inst's IfcRelNests parent: IfcRelNests(...,RelatingObject=4,
// RelatedObjects=5) — same attribute layout as IfcRelAggregates.
func getNest(f *step.File, inst *step.Instance) *step.Instance {
	return memberOf(f, inst, "IfcRelNests", attrRel5, attrRel4) // RelatingObject
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

// memberOf returns the relating side of the first relation of class that
// names inst in its related list at memberAttr and has a relating side at
// relatingAttr. The inverse index records the attribute of each reference, so
// membership is not a scan of the list: every member of an N-member relation
// scanning it would cost N².
func memberOf(f *step.File, inst *step.Instance, class string, memberAttr, relatingAttr int) *step.Instance {
	for _, r := range f.InverseIndices(inst) {
		if r.AttrIndex != memberAttr || !r.From.IsA(class) {
			continue
		}
		if relating, ok := r.From.Ref(relatingAttr); ok {
			return relating
		}
	}
	return nil
}

// containerOf ports ifcopenshell.util.element.get_container (recursive): if
// inst is directly contained via IfcRelContainedInSpatialStructure, return
// the RelatingStructure; otherwise recurse on get_parent (aggregate, nest,
// filled-void, voided-element), matching ifcopenshell's chain: e.g. a door
// resolves door→(filled_void)→opening→(voided_element)→wall→(contained)→
// storey. depth guards against malformed/cyclic IFC.
func containerOf(f *step.File, inst *step.Instance, depth int) *step.Instance {
	return newContainers(f).of(inst, depth)
}

// containers remembers each instance's container for one pass over a file.
// Many elements can climb to one parent, and finding the parent's container
// reads all of its references: without the memo, N children of one parent
// cost N².
type containers struct {
	f    *step.File
	memo map[int]climb
}

// climb is a container found hops parents above an instance, or the end of
// its parent chain hops parents above it when c is nil.
type climb struct {
	c    *step.Instance
	hops int
}

func newContainers(f *step.File) *containers {
	return &containers{f: f, memo: map[int]climb{}}
}

func (r *containers) of(inst *step.Instance, depth int) *step.Instance {
	c, _ := r.climb(inst, depth)
	return c
}

// climb returns inst's container and whether the result holds at any depth
// greater than the hops it took. A result cut short by depth, as in a cycle,
// is not remembered: a call with more depth could see further.
func (r *containers) climb(inst *step.Instance, depth int) (*step.Instance, bool) {
	if depth <= 0 {
		return nil, false
	}
	if m, ok := r.memo[inst.ID()]; ok && m.hops < depth {
		return m.c, true
	}
	if s := memberOf(r.f, inst, "IfcRelContainedInSpatialStructure", attrRel4, attrRel5); s != nil { // RelatingStructure
		r.memo[inst.ID()] = climb{c: s}
		return s, true
	}
	p := getParent(r.f, inst)
	if p == nil {
		r.memo[inst.ID()] = climb{}
		return nil, true
	}
	c, whole := r.climb(p, depth-1)
	if !whole {
		return nil, false
	}
	r.memo[inst.ID()] = climb{c: c, hops: r.memo[p.ID()].hops + 1}
	return c, true
}

// Storey ports decompose._storey: resolves inst's enclosing IfcBuildingStorey
// name via containerOf (ifcopenshell get_container, recursing through
// nesting/filled-void/voiding when inst isn't directly contained), climbing
// up to 8 hops.
func Storey(f *step.File, inst *step.Instance) string {
	return newContainers(f).storey(inst)
}

func (r *containers) storey(inst *step.Instance) string {
	c := r.of(inst, 32)
	for i := 0; i < 8 && c != nil; i++ {
		if c.IsA("IfcBuildingStorey") {
			return strVal(c, attrName)
		}
		c = r.of(c, 32)
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

// maxMaterialDepth bounds materialLeaves' recursion. A schema-valid material
// chain is at most four levels deep (usage -> set -> layer -> material); the
// bound guards against a crafted acyclic but pathologically deep chain
// overflowing the goroutine stack, which recover() cannot catch. Cycles are
// cut separately, by the on-path set in materialLeaves.
const maxMaterialDepth = 64

// maxMaterialVisits bounds the nodes one materialLeaves call visits. The
// on-path cycle guard lets a shared wrapper be walked once per reference (real
// exporters list one IfcMaterialLayer on both faces of a wall), so a crafted
// chain of lists each holding the next twice would otherwise take 2^depth
// visits. A real element needs tens.
const maxMaterialVisits = 4096

// materialLeaves resolves m (e.g. a layer set usage / layer set / material
// list / profile set / constituent set) down to its IfcMaterial leaves,
// walking the declared EXPRESS LIST order rather than the forward reference
// graph (graph-DFS order does not preserve IfcMaterialLayerSet.MaterialLayers
// order and can return the wrong layer's material first). Mirrors
// ifcopenshell.util.element.get_materials.
//
// A reference back to a wrapper already on the current path is a cycle and is
// skipped; every other repeat is walked again, so a material reached through
// two layers (or one layer listed twice) is listed once per reference. The walk
// stops at maxMaterialDepth / maxMaterialVisits and returns what it collected.
func materialLeaves(f *step.File, m *step.Instance) []*step.Instance {
	var out []*step.Instance
	onPath := map[int]bool{}
	visits := 0
	var walk func(m *step.Instance, depth int)
	ref := func(m *step.Instance, idx, depth int) {
		if r, ok := m.Ref(idx); ok { // optional/nullable on every wrapper
			walk(r, depth+1)
		}
	}
	list := func(m *step.Instance, idx, depth int) {
		v, ok := m.Get(idx)
		if !ok || v.Kind() != step.KindList {
			return
		}
		for _, item := range v.List() {
			if item.Kind() == step.KindRef && item.Ref() != nil {
				walk(item.Ref(), depth+1)
			}
		}
	}
	walk = func(m *step.Instance, depth int) {
		visits++
		if depth > maxMaterialDepth || visits > maxMaterialVisits {
			return
		}
		if m.IsA("IfcMaterial") {
			out = append(out, m)
			return
		}
		if onPath[m.ID()] {
			return
		}
		onPath[m.ID()] = true
		defer delete(onPath, m.ID())
		switch {
		case m.IsA("IfcMaterialLayerSetUsage"):
			ref(m, attrForLayerSet, depth)
		case m.IsA("IfcMaterialLayerSet"):
			list(m, attrMaterialLayers, depth)
		case m.IsA("IfcMaterialLayer"):
			ref(m, attrLayerMaterial, depth)
		case m.IsA("IfcMaterialList"):
			list(m, attrMaterialListMaterials, depth)
		case m.IsA("IfcMaterialProfileSetUsage"):
			ref(m, attrForProfileSet, depth)
		case m.IsA("IfcMaterialProfileSet"):
			list(m, attrMaterialProfiles, depth)
		case m.IsA("IfcMaterialProfile"):
			ref(m, attrProfileMaterial, depth)
		case m.IsA("IfcMaterialConstituentSet"):
			list(m, attrMaterialConstituents, depth)
		case m.IsA("IfcMaterialConstituent"):
			ref(m, attrConstituentMaterial, depth)
		}
	}
	walk(m, 0)
	return out
}

// IsExternal returns the tri-state *Common.IsExternal pset value: nil when no
// *Common pset carries a boolean IsExternal property (including the EXPRESS
// LOGICAL .U. "unknown" case, which nominalGoValue already maps to nil).
func IsExternal(f *step.File, inst *step.Instance) *bool {
	return isExternalIn(Psets(f, inst, false))
}

func isExternalIn(psets map[string]map[string]any) *bool {
	for name, props := range psets {
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
	if v.Kind() == step.KindTyped && len(v.List()) == 1 {
		inner = v.List()[0]
	}
	switch inner.Kind() {
	case step.KindBool:
		return inner.Bool()
	case step.KindFloat:
		return inner.Float()
	case step.KindInt:
		return inner.Int()
	case step.KindString, step.KindEnum:
		return inner.Str()
	case step.KindLogical:
		return nil // .U. unknown
	}
	return nil
}
