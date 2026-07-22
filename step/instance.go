package step

import "strings"

// Instance is a parsed STEP entity instance: a schema-agnostic entity_instance
// (ported from ifcopenshell). Attribute access is positional — access by NAME is a
// schema concern layered on by a later component. The type keyword and argument
// list come straight from the SPF record.
type Instance struct {
	id   uint32
	typ  string // interned, upper-cased type keyword (e.g. "IFCWALL")
	args []Value
	file *File
}

// ID returns the STEP instance name (#id). The zero-value Instance reports 0.
func (i *Instance) ID() int { return int(i.id) }

// Type returns the upper-cased entity type keyword. It is the schema-agnostic
// equivalent of ifcopenshell's is_a() with no arguments — an exact type string,
// not a supertype-chain check.
func (i *Instance) Type() string { return i.typ }

// Len returns the number of positional attributes.
func (i *Instance) Len() int { return len(i.args) }

// Get returns the attribute at idx and whether idx is in range.
func (i *Instance) Get(idx int) (Value, bool) {
	if idx < 0 || idx >= len(i.args) {
		return Value{}, false
	}
	return i.args[idx], true
}

// Args returns the underlying attribute slice. Callers must not mutate it.
func (i *Instance) Args() []Value { return i.args }

// File returns the owning file.
func (i *Instance) File() *File { return i.file }

// Ref returns the resolved instance referenced by attribute idx, if that attribute
// is a resolved reference (#id). ok is false when idx is out of range, not a ref,
// or a dangling ref whose target was missing.
func (i *Instance) Ref(idx int) (*Instance, bool) {
	v, ok := i.Get(idx)
	if !ok || v.Kind != KindRef || v.Ref == nil {
		return nil, false
	}
	return v.Ref, true
}

// IsA reports whether this instance's exact type keyword equals keyword
// (case-insensitive). For a complex instance it matches any of its part types.
// This is exact-only: supertype checks (IfcWall IS-A IfcElement) require the
// EXPRESS schema and are out of scope here.
func (i *Instance) IsA(keyword string) bool {
	if strings.EqualFold(i.typ, keyword) {
		return true
	}
	if i.file != nil {
		for _, t := range i.file.complexTypes[i.id] {
			if strings.EqualFold(t, keyword) {
				return true
			}
		}
	}
	return false
}

// Walk applies fn to every value in every attribute, pre-order (nested lists and
// typed values included).
func (i *Instance) Walk(fn func(Value)) {
	for _, a := range i.args {
		a.Walk(fn)
	}
}
