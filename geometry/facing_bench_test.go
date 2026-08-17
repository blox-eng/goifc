package geometry

import (
	"strconv"
	"testing"
)

// benchModel is a floor plate of n elements laid out on a 4 m grid: three
// quarters walls, one quarter slabs, with mid-heights jittered across many
// distinct slice keys.
//
// Both properties are the point. Bands are keyed on quantized element
// MID-HEIGHT, not on storey, so a real model with walls, parapets, upstands and
// slabs at assorted heights produces far more bands than it has storeys. And a
// slab has no facade at all, so the grid its band would need is built only to
// be declined.
func benchModel(n int) []Element {
	side := 1
	for side*side < n {
		side++
	}
	elems := make([]Element, 0, n)
	for i := 0; i < n; i++ {
		x := float64(i%side) * 4
		y := float64(i/side) * 4
		if i%4 == 3 {
			z := 3 + float64(i%89)*0.1
			elems = append(elems, namedWall("f"+strconv.Itoa(i),
				v3{x, y, z}, v3{x + 3.7, y + 3.7, z + 0.2}))
			continue
		}
		top := 3 + float64(i%97)*0.1
		elems = append(elems, namedWall("w"+strconv.Itoa(i), v3{x, y, 0}, v3{x + 3.7, y + 0.3, top}))
	}
	return elems
}

func BenchmarkBuildFacings(b *testing.B) {
	for _, n := range []int{41, 305, 1010} {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			elems := benchModel(n)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				BuildFacings(elems)
			}
		})
	}
}
