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

// tessellate subdivides each of e's triangles into 4, leaving the SHAPE
// untouched and changing only the vertex count.
//
// Why the bench family needs this: benchModel's elements are 12-triangle boxes,
// and on those the vertex transform is nearly free, so a benchmark built only
// from them measures grid rasterization and is blind to anything scaling with
// mesh size. A real building is not like that — kb645.ifc averages ~1,769
// triangles per element, ~147x a box. A change worth 7x on that model moved the
// box-only benchmark by 2.7%. A benchmark that cannot see the regime it is
// meant to defend is not defending it.
func tessellate(e Element, depth int) Element {
	for d := 0; d < depth; d++ {
		verts := append([]float32(nil), e.Verts...)
		tris := make([]uint32, 0, len(e.Tris)*4)
		mid := func(a, b uint32) uint32 {
			idx := uint32(len(verts) / 3)
			for k := uint32(0); k < 3; k++ {
				verts = append(verts, (e.Verts[3*a+k]+e.Verts[3*b+k])/2)
			}
			return idx
		}
		for i := 0; i+2 < len(e.Tris); i += 3 {
			a, b2, c := e.Tris[i], e.Tris[i+1], e.Tris[i+2]
			ab, bc, ca := mid(a, b2), mid(b2, c), mid(c, a)
			tris = append(tris, a, ab, ca, ab, b2, bc, ca, bc, c, ab, bc, ca)
		}
		e.Verts, e.Tris = verts, tris
	}
	return e
}

// BenchmarkBuildFacingsDense is BenchmarkBuildFacings over elements with a
// realistic triangle count, so the mesh-size-sensitive half of the cost is
// represented. Keep both: they defend different things.
func BenchmarkBuildFacingsDense(b *testing.B) {
	for _, n := range []int{41, 305} {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			elems := benchModel(n)
			for i := range elems {
				elems[i] = tessellate(elems[i], 3) // 12 -> 768 tris per element
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				BuildFacings(elems)
			}
		})
	}
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
