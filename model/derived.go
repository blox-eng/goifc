package model

// Quantity provenance tiers, in descending trust order. "qto" = authored
// IfcElementQuantity (net, exact); "geometry" = derived from the tessellated
// proxy mesh (gross, bounding); "none" = neither available (a fully-empty Qto —
// NEVER a fabricated 0.0).
const (
	QuantitySourceQto      = "qto"
	QuantitySourceGeometry = "geometry"
	QuantitySourceNone     = "none"
)

// ApplyDerivedQuantities back-fills the tier-2 (geometry-derived) quantities for
// every element still on quantity_source="none" — i.e. the semantic Qto tier
// found no authored quantities. derived is keyed by GlobalID (produced by
// geometry.Scene.DerivedQuantities, which lives in the geometry package so this
// one stays free of a geometry import / cycle).
//
// Tiering guarantees:
//   - "qto" elements are never touched — authored NET quantities always outrank
//     geometry-derived GROSS ones (prefer-net-over-gross).
//   - an element whose derived entry is missing or empty stays "none" — a missing
//     quantity is never coerced to a phantom 0.0.
//   - only elements upgraded here flip to source="geometry".
func (r *Result) ApplyDerivedQuantities(derived map[string]Quantities) {
	for i := range r.Elements {
		e := &r.Elements[i]
		if e.QuantitySource != QuantitySourceNone {
			continue
		}
		q, ok := derived[e.GlobalID]
		if !ok || q.IsEmpty() {
			continue
		}
		e.Qto = q
		e.QuantitySource = QuantitySourceGeometry
	}
}

// IsEmpty reports whether every quantity is absent (all-nil) — the guard that
// keeps ApplyDerivedQuantities from flipping an element to "geometry" without a
// single real value behind it.
func (q Quantities) IsEmpty() bool {
	return q.Area == nil && q.Volume == nil && q.Length == nil &&
		q.Width == nil && q.Height == nil && q.Perimeter == nil
}
