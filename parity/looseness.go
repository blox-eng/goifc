package parity

import "sort"

// boxPair is one element's goifc box against the oracle's.
type boxPair struct {
	Got  AABB // goifc
	Want AABB // ifcopenshell
}

// Looseness summarizes how much larger goifc's bounds are than the oracle's.
// The ratio is the product metric: closing a geometry gap moves it toward 1.0.
type Looseness struct {
	Compared   int     // elements with a usable ratio
	Degenerate int     // elements whose ORACLE box has zero volume, so no ratio exists
	Collapsed  int     // elements whose GOIFC box has zero volume: flat, inverted, or empty
	P50        float64 // median ratio
	P90        float64
	Max        float64
}

// summarize computes the ratio distribution over the pairs where a ratio means
// something, counting the two ways it cannot mean anything rather than dropping
// them silently.
//
// A zero-volume ORACLE box would divide to +Inf and destroy every percentile.
//
// A zero-volume GOIFC box is the opposite trap: it divides to exactly 0, which
// is a perfectly well-formed ratio and the worst possible outcome — a flat,
// inverted, or empty box where a solid should be (geometry.Build leaves
// BBoxMin == BBoxMax for a zero-vertex element). Left in, it drags p50 and p90
// DOWN, toward 1.0, so a regression that collapses elements reads on the
// published page as an improvement. Gate 1 still catches them; this keeps the
// page from contradicting it.
func summarize(pairs []boxPair) Looseness {
	var l Looseness
	ratios := make([]float64, 0, len(pairs))
	for _, p := range pairs {
		wv := Volume(p.Want)
		if wv <= 0 {
			l.Degenerate++
			continue
		}
		gv := Volume(p.Got)
		if gv <= 0 {
			l.Collapsed++
			continue
		}
		ratios = append(ratios, gv/wv)
	}
	l.Compared = len(ratios)
	if l.Compared == 0 {
		return l
	}
	sort.Float64s(ratios)
	l.P50 = percentile(ratios, 0.50)
	l.P90 = percentile(ratios, 0.90)
	l.Max = ratios[len(ratios)-1]
	return l
}

// percentile returns the value at q in a sorted slice, nearest-rank.
func percentile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	i := int(q * float64(len(sorted)-1))
	if i < 0 {
		i = 0
	}
	if i >= len(sorted) {
		i = len(sorted) - 1
	}
	return sorted[i]
}

// MeasureLooseness builds a model and summarizes its ratio distribution against
// the oracle.
func MeasureLooseness(name string) (Looseness, error) {
	oracle, err := LoadOracle(name)
	if err != nil {
		return Looseness{}, err
	}
	sc, err := SceneOf(name)
	if err != nil {
		return Looseness{}, err
	}
	pairs := make([]boxPair, 0, len(sc.Elements))
	for _, e := range sc.Elements {
		want, ok := oracle[e.GlobalID]
		if !ok {
			continue
		}
		pairs = append(pairs, boxPair{Got: ElementBox(e), Want: want})
	}
	return summarize(pairs), nil
}
