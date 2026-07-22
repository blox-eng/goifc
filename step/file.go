package step

import (
	"iter"
	"strings"
)

// TraverseOrder selects the visitation order of File.Traverse.
type TraverseOrder int

const (
	DepthFirst   TraverseOrder = iota // pre-order depth-first (default)
	BreadthFirst                      // level-order breadth-first
)

// Unbounded is the maxLevels value for an unbounded File.Traverse.
const Unbounded = -1

// Header holds the ISO-10303-21 HEADER section fields.
type Header struct {
	Description         []string // FILE_DESCRIPTION descriptions
	ImplementationLevel string   // FILE_DESCRIPTION implementation_level
	Name                []string // FILE_NAME fields (raw, in declaration order)
	Schema              []string // FILE_SCHEMA identifiers, e.g. ["IFC2X3"]
}

// InverseRef records one referrer of an instance: the referring instance and the
// top-level attribute index on it that holds the reference. This is the raw
// referrer graph; projecting it into named IFC inverse attributes is a schema
// concern for a later component.
type InverseRef struct {
	From      *Instance
	AttrIndex int
}

// File is a parsed STEP model: the header plus a navigable entity graph. Lookups
// (ByID/ByType), the forward reference graph (resolved on Value.Ref), and the
// inverse index are all pure-SPF — no EXPRESS schema required.
type File struct {
	Head    Header
	byID    map[uint32]*Instance
	byType  map[string][]*Instance
	inverse map[uint32][]InverseRef
	order   []uint32
	// complexTypes holds the additional part type keywords of ISO-10303-21 complex
	// instances (#id=(TYPEA(...)TYPEB(...))), keyed by id. Nil/absent for the common
	// simple instance; Instance.Type reports the first part, IsA matches any part.
	complexTypes map[uint32][]string
	warnings     []string
}

// SchemaID returns the first FILE_SCHEMA identifier (e.g. "IFC2X3"), or "".
func (f *File) SchemaID() string {
	if len(f.Head.Schema) == 0 {
		return ""
	}
	return f.Head.Schema[0]
}

// Len returns the number of DATA-section instances.
func (f *File) Len() int { return len(f.order) }

// Warnings returns non-fatal issues encountered during parse (e.g. dangling
// references to missing instances), mirroring ifcopenshell's SYN diagnostics.
func (f *File) Warnings() []string { return f.warnings }

// ByID looks up an instance by its STEP name (#id).
func (f *File) ByID(id int) (*Instance, bool) {
	inst, ok := f.byID[uint32(id)]
	return inst, ok
}

// ByType returns all instances whose exact type keyword matches (case-insensitive).
// Subtype expansion (all IfcElement subtypes) requires the schema and is out of
// scope here.
func (f *File) ByType(keyword string) []*Instance {
	return f.byType[strings.ToUpper(keyword)]
}

// All returns an iterator over every instance in source (insertion) order. It
// allocates nothing and supports early termination:
//
//	for inst := range f.All() {
//		...
//	}
func (f *File) All() iter.Seq[*Instance] {
	return func(yield func(*Instance) bool) {
		for _, id := range f.order {
			if !yield(f.byID[id]) {
				return
			}
		}
	}
}

// Inverse returns the distinct instances that reference inst via any forward
// attribute (the raw referrer set). Order follows first-seen referrer.
func (f *File) Inverse(inst *Instance) []*Instance {
	refs := f.inverse[inst.id]
	out := make([]*Instance, 0, len(refs))
	seen := make(map[uint32]bool, len(refs))
	for _, r := range refs {
		if seen[r.From.id] {
			continue
		}
		seen[r.From.id] = true
		out = append(out, r.From)
	}
	return out
}

// InverseIndices returns every (referrer, attribute-index) pair pointing at inst,
// including multiple entries for one referrer that references inst more than once.
// This is what a schema layer filters to build named inverse attributes.
func (f *File) InverseIndices(inst *Instance) []InverseRef {
	return f.inverse[inst.id]
}

// TotalInverses returns the count of distinct instances referencing inst.
func (f *File) TotalInverses(inst *Instance) int {
	refs := f.inverse[inst.id]
	seen := make(map[uint32]bool, len(refs))
	for _, r := range refs {
		seen[r.From.id] = true
	}
	return len(seen)
}

// Traverse returns the forward transitive closure of inst (inst included),
// following resolved references through attributes, nested lists, and typed
// values. maxLevels is the depth limit: Unbounded (-1) for no limit, 0 for just
// inst. order selects DepthFirst or BreadthFirst. Each instance appears once.
//
// Bounded traversal is depth-correct regardless of order: a node reachable within
// maxLevels is always included even if a longer path to it is explored first
// (bestDepth tracks the shortest known depth and allows a node to be re-expanded
// when a shorter path reaches it).
func (f *File) Traverse(inst *Instance, maxLevels int, order TraverseOrder) []*Instance {
	bestDepth := map[uint32]int{inst.id: 0}
	inOut := map[uint32]bool{inst.id: true}
	out := []*Instance{inst}
	type item struct {
		inst  *Instance
		depth int
	}
	queue := []item{{inst, 0}}
	for len(queue) > 0 {
		var cur item
		if order == BreadthFirst {
			cur, queue = queue[0], queue[1:]
		} else {
			cur, queue = queue[len(queue)-1], queue[:len(queue)-1]
		}
		if maxLevels >= 0 && cur.depth >= maxLevels {
			continue
		}
		nd := cur.depth + 1
		cur.inst.Walk(func(v Value) {
			if v.Kind != KindRef || v.Ref == nil {
				return
			}
			if bd, ok := bestDepth[v.Ref.id]; ok && bd <= nd {
				return // already reached at an equal-or-shorter depth
			}
			bestDepth[v.Ref.id] = nd
			if !inOut[v.Ref.id] {
				inOut[v.Ref.id] = true
				out = append(out, v.Ref)
			}
			queue = append(queue, item{v.Ref, nd})
		})
	}
	return out
}
