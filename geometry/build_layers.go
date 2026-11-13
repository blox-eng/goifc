package geometry

import (
	"errors"
	"fmt"
	"math"
)

// maxLayers caps the layers of one element: each layer is a mesh of its own.
const maxLayers = 16

// checkLayers refuses a layer list that is empty, longer than maxLayers, or
// holds a thickness out of bounds, and returns the summed thickness.
func checkLayers(layers []float64) (total float64, err error) {
	if len(layers) == 0 {
		return 0, errors.New("no layers: a layered element needs at least one layer")
	}
	if len(layers) > maxLayers {
		return 0, fmt.Errorf("%d layers, more than %d", len(layers), maxLayers)
	}
	for k, t := range layers {
		if err := checkDim(fmt.Sprintf("layer %d: thickness", k), t); err != nil {
			return 0, err
		}
		total += t
	}
	return total, checkDim("thickness", total)
}

// BuildSlabLayers builds a slab in layers, top first: layer k's top face lies
// at z minus the layers above it. Each layer is a Slab with its own volume;
// all share the outline's area and perimeter.
// Invalid input returns an error and nothing else.
func BuildSlabLayers(outline [][2]float64, z float64, layers []float64) ([]Slab, error) {
	if err := checkOutline(outline); err != nil {
		return nil, err
	}
	if err := checkOrigin("z", z); err != nil {
		return nil, err
	}
	if _, err := checkLayers(layers); err != nil {
		return nil, err
	}
	ring, cap, err := slabCap(outline)
	if err != nil {
		return nil, err
	}
	slabs := make([]Slab, len(layers))
	top := z
	for k, t := range layers {
		slabs[k] = slabAt(outline, ring, cap, top, t)
		top -= t
	}
	return slabs, nil
}

// onBoundary is how close a niche's back must lie to a layer boundary to be
// on it: well under minDim, so a depth typed as a sum of layers lands on it.
const onBoundary = 1e-9

// layerPart is one layer of a wall, ready to mesh.
type layerPart struct {
	span      wallSpan // in the layer's own u, from its measured-side face start
	shift     float64  // where that face starts, in the whole wall's u
	depth     float64  // d_k: the layer's measured-side face, from the measured line
	thickness float64
	ops       []Opening // the openings as they cut this layer, in its own u
	backs     float64   // m², niche backs that end exactly on this layer's measured face
}

// splitLayers splits a checked wall into its layers. The wall's end lines run
// straight from the measured face to the opposite face, so a face at depth d
// spans [s(d), e(d)], linear in d.
func splitLayers(span wallSpan, layers []float64, ops []Opening) []layerPart {
	var total float64
	for _, t := range layers {
		total += t
	}
	start := func(d float64) float64 { return span.otherStart * d / total }
	end := func(d float64) float64 { return span.length + (span.otherEnd-span.length)*d/total }
	parts := make([]layerPart, len(layers))
	d0 := 0.0
	for k, t := range layers {
		d1 := d0 + t
		s0, e0, s1, e1 := start(d0), end(d0), start(d1), end(d1)
		if k == 0 {
			s0, e0 = 0, span.length
		}
		if k == len(layers)-1 {
			d1, s1, e1 = total, span.otherStart, span.otherEnd
		}
		p := layerPart{
			span:      wallSpan{length: e0 - s0, otherStart: s1 - s0, otherEnd: e1 - s0},
			shift:     s0,
			depth:     d0,
			thickness: t,
		}
		for _, o := range ops {
			c := o.cut(total)
			switch {
			case c >= d1-onBoundary:
				o.Depth = 0
			case c > d0+onBoundary:
				o.Depth = c - d0
			case k > 0 && math.Abs(c-d0) <= onBoundary:
				p.backs += o.Width * o.Height
				continue
			default:
				continue
			}
			o.Offset -= s0
			p.ops = append(p.ops, o)
		}
		parts[k] = p
		d0 = d1
	}
	return parts
}

// checkLayerSlivers refuses each niche whose back lies under 1 mm from an
// inner layer boundary but not on it.
func checkLayerSlivers(wall int, layers []float64, ops []Opening) error {
	var errs []error
	for i, o := range ops {
		if o.Depth == 0 {
			continue
		}
		d := 0.0
		for _, t := range layers[:len(layers)-1] {
			d += t
			if g := math.Abs(o.Depth - d); g > onBoundary && g < minDim {
				errs = append(errs, &PlacementError{Wall: wall, Opening: i, Reason: ReasonLayerSliver})
				break
			}
		}
	}
	return errors.Join(errs...)
}

// layerCells is the mesh grid of every layer of a wall.
func layerCells(parts []layerPart, height float64) int {
	n := 0
	for _, p := range parts {
		n += gridCells(p.span, height, p.ops)
	}
	return n
}

// buildLayer meshes one layer and measures it.
func buildLayer(f wallFrame, origin [3]float64, height float64, p layerPart) Wall {
	pts, tris := meshWall(p.span, height, p.thickness, p.ops)
	for i := range pts {
		pts[i][0] += p.shift
		pts[i][1] += p.depth
	}
	verts, tris := placeWall(f, pts, tris)
	q := wallQuantities(p.span, height, p.thickness, p.ops)
	q.MeasuredNet -= p.backs
	q.NicheBack += p.backs
	at := [3]float64{
		origin[0] + p.shift*f.u[0] + p.depth*f.g[0],
		origin[1] + p.shift*f.u[1] + p.depth*f.g[1],
		origin[2],
	}
	return Wall{
		Element:    newElement(verts, tris, origin),
		Quantities: q,
		Measured:   measuredFace(at, f, p.span.length, height, p.ops),
	}
}

// wallLayers builds every layer of a checked wall.
func wallLayers(f wallFrame, origin [3]float64, span wallSpan, height float64, layers []float64, ops []Opening) []Wall {
	parts := splitLayers(span, layers, ops)
	walls := make([]Wall, len(parts))
	for k, p := range parts {
		walls[k] = buildLayer(f, origin, height, p)
	}
	return walls
}

// BuildWallLayers builds a straight wall in layers, measured face first. Each
// layer is a Wall with its own faces, quantities and mesh; the layers' sum is
// the wall's thickness. Openings are checked once, on the whole wall, so a
// layered wall refuses what BuildWall refuses with the summed thickness, and
// also a niche whose back would leave a layer sliver (ReasonLayerSliver).
//
// A through opening cuts every layer. A niche cuts the layers it passes
// through, is a niche in the layer where its back falls, and leaves the rest
// whole; a back exactly on a boundary is the next layer's NicheBack.
func BuildWallLayers(edge [2][2]float64, base, height float64, layers []float64, side Side, openings []Opening) ([]Wall, error) {
	total, err := checkLayers(layers)
	if err != nil {
		return nil, err
	}
	span, err := checkWall(edge, base, height, total, side, openings)
	if err := joinRefusals(err, checkLayerSlivers(0, layers, openings)); err != nil {
		return nil, err
	}
	if layerCells(splitLayers(span, layers, openings), height) > maxMeshCells {
		return nil, errTooMuchMesh
	}
	return wallLayers(newWallFrame(edge[0], edge[1], side), wallOrigin(edge[0], base), span, height, layers, openings), nil
}

// joinRefusals joins placement refusals flat, whole-wall ones first. A plain
// error (bad input) stops alone.
func joinRefusals(whole, slivers error) error {
	var pe *PlacementError
	if whole != nil && !errors.As(whole, &pe) {
		return whole
	}
	return errors.Join(append(unjoin(whole), unjoin(slivers)...)...)
}

// unjoin returns the errors an errors.Join holds, or err alone.
func unjoin(err error) []error {
	if err == nil {
		return nil
	}
	if j, ok := err.(interface{ Unwrap() []error }); ok {
		return j.Unwrap()
	}
	return []error{err}
}
