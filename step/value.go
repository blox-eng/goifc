package step

import (
	"math"
	"strconv"
)

// Kind tags the variant of a parsed STEP attribute value. It mirrors the runtime
// categories ifcopenshell distinguishes from the SPF token alone (no schema): the
// declared EXPRESS type is a separate, schema-driven concern layered on later.
type Kind uint8

const (
	KindNull    Kind = iota // $  (unset / omitted optional)
	KindDerived             // *  (value derived in a supertype)
	KindInt                 // integer literal        -> Int
	KindFloat               // real literal           -> Float
	KindString              // '...' (decoded)        -> Str
	KindEnum                // .LABEL.                -> Str (label, no dots)
	KindBool                // .T./.F.                -> Bool (EXPRESS BOOLEAN)
	KindLogical             // .U.                    -> (no payload) EXPRESS LOGICAL "unknown", distinct from false
	KindBinary              // "0..." binary          -> Str (raw hex/bit text)
	KindRef                 // #id                    -> RefID, Ref (nil when the target is missing)
	KindList                // (...) aggregate        -> List
	KindTyped               // KEYWORD(inner)         -> Str (keyword) + List (inner args)
)

// String returns the kind name, so kinds render readably in error messages.
func (k Kind) String() string {
	switch k {
	case KindNull:
		return "null"
	case KindDerived:
		return "derived"
	case KindInt:
		return "int"
	case KindFloat:
		return "float"
	case KindString:
		return "string"
	case KindEnum:
		return "enum"
	case KindBool:
		return "bool"
	case KindLogical:
		return "logical"
	case KindBinary:
		return "binary"
	case KindRef:
		return "ref"
	case KindList:
		return "list"
	case KindTyped:
		return "typed"
	default:
		return "Kind(" + strconv.Itoa(int(k)) + ")"
	}
}

// Value is a parsed STEP attribute value: a tagged union read through methods.
// Only the accessor named for a Kind is meaningful; the others return zero.
//
// A Value is a 24-byte handle into its File, not a self-contained tree: strings
// live in a string arena, list members in a value slab, and a reference is the
// target's id, looked up on access. A file parsed in parallel has one arena and
// slab per chunk, named by the top bits of the handle's offset. A large model
// therefore holds a handful of big allocations instead of millions of small
// ones. The only pointer in a Value is its File, so the garbage collector has
// no small objects to chase, only that one pointer per value to read.
type Value struct {
	x    uint64 // int64 / float64 bits / bool / ref id / slab<<slabShift | string or list offset
	n    uint32 // string or list length
	kind Kind
	f    *File
}

// slabShift splits a string or list handle into a slab number and an offset
// within it: 16 bits of slab, 48 of offset.
const (
	slabShift = 48
	offMask   = 1<<slabShift - 1
)

// Kind reports which variant v holds, and so which accessor is meaningful.
func (v Value) Kind() Kind { return v.kind }

func slabRef(slab uint16, off int) uint64 { return uint64(slab)<<slabShift | uint64(off) }

func (v Value) loc() (*slab, uint64) { return &v.f.slabs[v.x>>slabShift], v.x & offMask }

// Str returns the text of a KindString (decoded), KindEnum (label without the
// dots), KindBinary (raw digits) or the keyword of a KindTyped value.
func (v Value) Str() string {
	switch v.kind {
	case KindString, KindEnum, KindBinary:
		s, off := v.loc()
		return s.strs[off : off+uint64(v.n)]
	case KindTyped:
		s, off := v.loc()
		return s.vals[off].Str()
	}
	return ""
}

// List returns the members of a KindList, or the inner arguments of a
// KindTyped value. The slice is shared with the File: do not mutate it.
func (v Value) List() []Value {
	switch v.kind {
	case KindList:
		s, off := v.loc()
		return s.vals[off : off+uint64(v.n)]
	case KindTyped:
		s, off := v.loc()
		return s.vals[off+1 : off+uint64(v.n)]
	}
	return nil
}

// Ref returns the instance a KindRef points at, or nil when the value is not a
// reference or its target is missing from the file.
func (v Value) Ref() *Instance {
	if v.kind != KindRef || v.f == nil {
		return nil
	}
	return v.f.instance(uint32(v.x))
}

// RefID returns the #id a KindRef names, whether or not the target exists.
func (v Value) RefID() uint32 {
	if v.kind != KindRef {
		return 0
	}
	return uint32(v.x)
}

// Float returns a KindFloat's value.
func (v Value) Float() float64 {
	if v.kind != KindFloat {
		return 0
	}
	return math.Float64frombits(v.x)
}

// Int returns a KindInt's value.
func (v Value) Int() int64 {
	if v.kind != KindInt {
		return 0
	}
	return int64(v.x)
}

// Bool returns a KindBool's value (.T. is true). .U. is KindLogical, not false.
func (v Value) Bool() bool { return v.kind == KindBool && v.x != 0 }

// Walk applies fn to v and, pre-order, to every value nested within it (lists and
// typed-value inner args).
func (v Value) Walk(fn func(Value)) {
	fn(v)
	for _, c := range v.List() {
		c.Walk(fn)
	}
}
