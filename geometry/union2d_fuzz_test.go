package geometry

import (
	"math"
	"reflect"
	"testing"
)

// The decoder's bounds. They are here so a fuzz worker spends its budget on
// SHAPES rather than on one enormous ring: the sweep is quadratic in a
// polygon's vertex count, and a 60-second worker that runs three inputs has
// found nothing. Neither bound exists in the library itself — [UnionArea2D]
// refuses no polygon for being large.
const (
	maxFuzzPolys      = 8
	maxFuzzRingPoints = 64
)

// fuzzCoord maps a byte onto a COARSE quarter-metre grid in [-32, 31.75].
//
// The coarseness is the point. On a fine grid two random rings never share a
// vertex, never share an edge and never touch, so the fuzzer would explore
// only the easy interior of the input space and never the boundary cases that
// break a union: coincident vertices, collinear edges, a ring closing on
// itself, a hole landing exactly on its outer ring.
func fuzzCoord(b byte) float64 { return float64(int8(b)) / 4 }

// decodeFuzzPolys reads a byte string as polygons: per polygon a header byte
// whose low two bits are the hole count, then per ring a length byte and that
// many (x, y) byte pairs. A truncated tail yields a short ring rather than
// being discarded — a half-written outline is exactly the input a consumer
// will one day hand over.
func decodeFuzzPolys(b []byte) []Polygon2D {
	var out []Polygon2D
	for i := 0; i+1 < len(b) && len(out) < maxFuzzPolys; {
		nHoles := int(b[i] & 0x3)
		i++
		rings := make([][][2]float64, 0, 1+nHoles)
		for r := 0; r <= nHoles && i < len(b); r++ {
			n := int(b[i])
			i++
			if n > maxFuzzRingPoints {
				n = maxFuzzRingPoints
			}
			ring := make([][2]float64, 0, n)
			for k := 0; k < n && i+1 < len(b); k++ {
				ring = append(ring, [2]float64{fuzzCoord(b[i]), fuzzCoord(b[i+1])})
				i += 2
			}
			rings = append(rings, ring)
		}
		if len(rings) == 0 {
			break
		}
		// Nil rather than an empty slice when there are no holes, so a decoded
		// polygon is indistinguishable from the same one written by hand.
		var holes [][][2]float64
		if len(rings) > 1 {
			holes = rings[1:]
		}
		out = append(out, Polygon2D{Outer: rings[0], Holes: holes})
	}
	return out
}

// encodeFuzzPolys is decodeFuzzPolys' inverse, so a seed can be written as the
// shape it means instead of as a byte string nobody can read. addFuzzSeed
// checks the round trip, because a seed that decodes to something other than
// what it was written as is a seed for a case nobody is testing.
func encodeFuzzPolys(polys []Polygon2D) []byte {
	var b []byte
	for _, p := range polys {
		b = append(b, byte(len(p.Holes)&0x3))
		for _, r := range append([][][2]float64{p.Outer}, p.Holes...) {
			b = append(b, byte(len(r)))
			for _, q := range r {
				b = append(b, byte(int8(math.Round(q[0]*4))), byte(int8(math.Round(q[1]*4))))
			}
		}
	}
	return b
}

func addFuzzSeed(f *testing.F, name string, polys []Polygon2D) {
	f.Helper()
	b := encodeFuzzPolys(polys)
	if got := decodeFuzzPolys(b); !reflect.DeepEqual(got, polys) {
		f.Fatalf("seed %q does not survive its own encoding: decoded %v, want %v", name, got, polys)
	}
	f.Add(b)
}

// FuzzUnionArea2D drives the union over arbitrary coordinate soup. This is an
// untrusted-input path in the same sense the parser is: a consumer may
// persist these outlines and read them back, so treat them as untrusted
// input -- a person could have written them by hand, or a migration could
// have truncated them.
//
// The contract asserted is narrow and absolute, because garbage in may
// legitimately measure to nothing:
//
//   - never panic, and never hang;
//   - never report ok with an area or a perimeter that is NaN, infinite or
//     negative — those are the values that would flow into a bill of
//     quantities and be believed;
//   - never report an area larger than the polygons' own areas summed, since a
//     union counts the overlap once and can only ever be smaller;
//   - UnionArea2D and UnionMeasure2D must agree, or a consumer's choice of
//     entry point silently changes the answer.
//
// The seeds are the degenerate cases by name. A fuzz target seeded only with a
// well-formed square spends its budget rediscovering that squares work.
func FuzzUnionArea2D(f *testing.F) {
	square := [][2]float64{{0, 0}, {2, 0}, {2, 2}, {0, 2}}
	addFuzzSeed(f, "a well-formed square", []Polygon2D{{Outer: square}})
	addFuzzSeed(f, "no polygons at all", nil)
	addFuzzSeed(f, "an empty ring", []Polygon2D{{Outer: [][2]float64{}}})
	addFuzzSeed(f, "a single point", []Polygon2D{{Outer: [][2]float64{{1, 1}}}})
	addFuzzSeed(f, "two points", []Polygon2D{{Outer: [][2]float64{{0, 0}, {1, 1}}}})
	addFuzzSeed(f, "collinear points", []Polygon2D{{Outer: [][2]float64{{0, 0}, {1, 1}, {2, 2}, {3, 3}}}})
	addFuzzSeed(f, "one point repeated", []Polygon2D{{Outer: [][2]float64{{1, 1}, {1, 1}, {1, 1}, {1, 1}}}})
	addFuzzSeed(f, "duplicate vertices in a real ring",
		[]Polygon2D{{Outer: [][2]float64{{0, 0}, {2, 0}, {2, 0}, {2, 2}, {0, 2}, {0, 2}}}})
	addFuzzSeed(f, "a self-touching bowtie",
		[]Polygon2D{{Outer: [][2]float64{{0, 0}, {2, 2}, {2, 0}, {0, 2}}}})
	addFuzzSeed(f, "a ring pinched to a point at its middle",
		[]Polygon2D{{Outer: [][2]float64{{0, 0}, {2, 0}, {1, 1}, {2, 2}, {0, 2}, {1, 1}}}})
	addFuzzSeed(f, "opposite windings on two polygons", []Polygon2D{
		{Outer: square},
		{Outer: [][2]float64{{1, 0}, {1, 2}, {3, 2}, {3, 0}}},
	})
	addFuzzSeed(f, "a hole inside its outer", []Polygon2D{{
		Outer: [][2]float64{{0, 0}, {4, 0}, {4, 4}, {0, 4}},
		Holes: [][][2]float64{{{1, 1}, {1, 2}, {2, 2}, {2, 1}}},
	}})
	addFuzzSeed(f, "a hole outside its outer", []Polygon2D{{
		Outer: square,
		Holes: [][][2]float64{{{8, 8}, {8, 9}, {9, 9}, {9, 8}}},
	}})
	addFuzzSeed(f, "a hole exactly on its outer", []Polygon2D{{
		Outer: square,
		Holes: [][][2]float64{square},
	}})
	addFuzzSeed(f, "two holes overlapping each other", []Polygon2D{{
		Outer: [][2]float64{{0, 0}, {4, 0}, {4, 4}, {0, 4}},
		Holes: [][][2]float64{
			{{1, 1}, {1, 3}, {3, 3}, {3, 1}},
			{{2, 2}, {2, 4}, {4, 4}, {4, 2}},
		},
	}})
	addFuzzSeed(f, "coincident polygons", []Polygon2D{{Outer: square}, {Outer: square}})
	addFuzzSeed(f, "edge-touching polygons", []Polygon2D{
		{Outer: square},
		{Outer: [][2]float64{{2, 0}, {4, 0}, {4, 2}, {2, 2}}},
	})
	f.Add([]byte{})
	f.Add([]byte{0})

	f.Fuzz(func(t *testing.T, b []byte) {
		polys := decodeFuzzPolys(b)
		area, ok := UnionArea2D(polys)
		mArea, per, mOK := UnionMeasure2D(polys)
		if ok != mOK || area != mArea {
			t.Fatalf("UnionArea2D = %v, %v but UnionMeasure2D = %v, %v", area, ok, mArea, mOK)
		}
		if !ok {
			return
		}
		for name, v := range map[string]float64{"area": area, "perimeter": per} {
			if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
				t.Fatalf("ok with %s = %v, on %d polygons", name, v, len(polys))
			}
		}
		var gross float64
		for _, p := range polys {
			gross += math.Abs(polygonArea2D(p.Outer))
		}
		// The absolute floor covers a union of rings whose own areas nearly
		// cancel; the relative term covers the ordinary case.
		if area > gross+1e-9+1e-9*gross {
			t.Fatalf("union area %v exceeds the polygons' own areas summed, %v", area, gross)
		}
	})
}
