package step

import "strconv"

// Kind tags the variant of a parsed STEP attribute value. It mirrors the runtime
// categories ifcopenshell distinguishes from the SPF token alone (no schema): the
// declared EXPRESS type is a separate, schema-driven concern layered on later.
type Kind uint8

const (
	KindNull    Kind = iota // $  (unset / omitted optional)
	KindDerived             // *  (value derived in a supertype)
	KindInt                 // integer literal        -> I
	KindFloat               // real literal           -> F
	KindString              // '...' (decoded)        -> Str
	KindEnum                // .LABEL.                -> Str (label, no dots)
	KindBool                // .T./.F.                -> B (EXPRESS BOOLEAN)
	KindLogical             // .U.                    -> (no payload) EXPRESS LOGICAL "unknown", distinct from false
	KindBinary              // "0..." binary          -> Str (raw hex/bit text)
	KindRef                 // #id                    -> RefID, Ref (nil until resolved)
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

// Value is a parsed STEP attribute value: an eager, in-memory tagged union
// (ported from ifcopenshell's attribute-value variant). It is a plain struct, not
// a boxed interface, so the millions of values in a large model avoid per-value
// heap allocation. Only the field(s) named for a Kind carry meaning.
//
// Fields are ordered largest-alignment-first so the struct packs to 72 bytes (vs
// 80 with a naive layout) — a ~10% cut on the dominant memory cost of a big model.
type Value struct {
	Str   string    // KindString / KindEnum / KindBinary / KindTyped(keyword)
	List  []Value   // KindList / KindTyped(inner args)
	Ref   *Instance // KindRef (resolved target; nil if the target is missing)
	F     float64   // KindFloat
	I     int64     // KindInt
	RefID uint32    // KindRef (target id, pre-resolution)
	Kind  Kind      // variant tag
	B     bool      // KindBool (.T. -> true, .F. -> false); .U. is KindLogical, not this field
}

// Walk applies fn to v and, pre-order, to every value nested within it (lists and
// typed-value inner args). It operates on value copies — to mutate stored values
// (e.g. resolving refs) the parser uses an internal by-pointer walk instead.
func (v Value) Walk(fn func(Value)) {
	fn(v)
	for _, c := range v.List {
		c.Walk(fn)
	}
}
