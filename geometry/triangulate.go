package geometry

import "math"

// triangulatePolygon ear-clips a simple 2D polygon (CCW or CW) into triangle
// indices into poly. Correct for concave polygons (a naive fan is not). Returns
// nil for < 3 points. Holes are not supported (v1: walls solid, profiles outer-only).
func triangulatePolygon(poly [][2]float64) []uint32 {
	n := len(poly)
	if n < 3 {
		return nil
	}
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	// Ensure CCW so the "convex vertex" test has a consistent sign.
	if polygonArea2D(poly) < 0 {
		for i, j := 0, n-1; i < j; i, j = i+1, j-1 {
			idx[i], idx[j] = idx[j], idx[i]
		}
	}
	var out []uint32
	guard := 0
	for len(idx) > 3 && guard < 10*n {
		guard++
		clipped := false
		for i := 0; i < len(idx); i++ {
			ia, ib, ic := idx[(i+len(idx)-1)%len(idx)], idx[i], idx[(i+1)%len(idx)]
			a, b, c := poly[ia], poly[ib], poly[ic]
			if cross2D(a, b, c) <= 0 {
				continue // reflex vertex, not an ear
			}
			isEar := true
			for _, j := range idx {
				if j == ia || j == ib || j == ic {
					continue
				}
				if pointInTri(poly[j], a, b, c) {
					isEar = false
					break
				}
			}
			if !isEar {
				continue
			}
			out = append(out, uint32(ia), uint32(ib), uint32(ic))
			idx = append(idx[:i], idx[i+1:]...)
			clipped = true
			break
		}
		if !clipped {
			break // degenerate polygon; emit what we have
		}
	}
	if len(idx) == 3 {
		out = append(out, uint32(idx[0]), uint32(idx[1]), uint32(idx[2]))
	}
	return out
}

// triangulateFace ear-clips a 3D planar face by projecting onto its dominant
// plane, returning triangle indices into loop.
func triangulateFace(loop []v3) []uint32 {
	if len(loop) < 3 {
		return nil
	}
	// Face normal via Newell's method → drop the largest-magnitude axis to project.
	var nx, ny, nz float64
	for i := range loop {
		j := (i + 1) % len(loop)
		nx += (loop[i][1] - loop[j][1]) * (loop[i][2] + loop[j][2])
		ny += (loop[i][2] - loop[j][2]) * (loop[i][0] + loop[j][0])
		nz += (loop[i][0] - loop[j][0]) * (loop[i][1] + loop[j][1])
	}
	ax, ay, az := math.Abs(nx), math.Abs(ny), math.Abs(nz)
	poly := make([][2]float64, len(loop))
	for i, p := range loop {
		switch {
		case ax >= ay && ax >= az:
			poly[i] = [2]float64{p[1], p[2]}
		case ay >= ax && ay >= az:
			poly[i] = [2]float64{p[2], p[0]}
		default:
			poly[i] = [2]float64{p[0], p[1]}
		}
	}
	tris := triangulatePolygon(poly)
	// triangulatePolygon winds its output CCW in the PROJECTED 2D plane. The
	// projection above drops the dominant axis without regard to the SIGN of that
	// axis's normal component, so a face whose true outward normal points along
	// the NEGATIVE dominant axis comes back wound inward. Orient the emitted
	// triangles to agree with the Newell face normal (brep.go already orients the
	// loop so that normal faces outward).
	if len(tris) >= 3 {
		n := v3{nx, ny, nz}
		// tris[0..2] is always a strictly-convex ear (triangulatePolygon skips reflex
		// vertices via cross2D <= 0), so its normal-sign test is reliable here.
		a, b, c := loop[tris[0]], loop[tris[1]], loop[tris[2]]
		e1 := v3{b[0] - a[0], b[1] - a[1], b[2] - a[2]}
		e2 := v3{c[0] - a[0], c[1] - a[1], c[2] - a[2]}
		if dotv(crossv(e1, e2), n) < 0 {
			for i := 0; i+2 < len(tris); i += 3 {
				tris[i+1], tris[i+2] = tris[i+2], tris[i+1]
			}
		}
	}
	return tris
}

// ensureCCW returns poly wound counter-clockwise, reversing it when it is
// clockwise (negative signed area). Callers that build geometry from a
// polygon's as-returned vertex order (e.g. extrudeSolid's side walls) need a
// consistent winding to agree with triangulatePolygon's own CCW normalization
// of the caps — otherwise one faces outward and the other inward.
func ensureCCW(poly [][2]float64) [][2]float64 {
	if polygonArea2D(poly) >= 0 {
		return poly
	}
	out := make([][2]float64, len(poly))
	for i, p := range poly {
		out[len(poly)-1-i] = p
	}
	return out
}

func polygonArea2D(p [][2]float64) float64 {
	var a float64
	for i := range p {
		j := (i + 1) % len(p)
		a += p[i][0]*p[j][1] - p[j][0]*p[i][1]
	}
	return a / 2
}

func cross2D(a, b, c [2]float64) float64 {
	return (b[0]-a[0])*(c[1]-a[1]) - (b[1]-a[1])*(c[0]-a[0])
}

func pointInTri(p, a, b, c [2]float64) bool {
	d1 := cross2D(p, a, b)
	d2 := cross2D(p, b, c)
	d3 := cross2D(p, c, a)
	hasNeg := d1 < 0 || d2 < 0 || d3 < 0
	hasPos := d1 > 0 || d2 > 0 || d3 > 0
	return !hasNeg || !hasPos
}
