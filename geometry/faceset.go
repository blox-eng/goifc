package geometry

import "github.com/blox-eng/goifc/step"

const (
	attrPointListCoords  = 0 // IfcCartesianPointList3D.CoordList
	attrFaceSetCoords    = 0 // IfcTessellatedFaceSet.Coordinates
	attrTfsCoordIndex    = 3 // IfcTriangulatedFaceSet.CoordIndex
	attrTfsPnIndex       = 4 // IfcTriangulatedFaceSet.PnIndex
	attrTinFlags         = 5 // IfcTriangulatedIrregularNetwork.Flags
	attrPfsFaces         = 2 // IfcPolygonalFaceSet.Faces
	attrPfsPnIndex       = 3 // IfcPolygonalFaceSet.PnIndex
	attrIndexedFaceCoord = 0 // IfcIndexedPolygonalFace.CoordIndex
)

// isTriangulatedFaceSet names the subtypes explicitly because IsA matches the
// exact keyword only. IfcTriangulatedIrregularNetwork (IFC4X3) appends a Flags
// attribute and otherwise shares the layout.
func isTriangulatedFaceSet(inst *step.Instance) bool {
	return inst.IsA("IfcTriangulatedFaceSet") || inst.IsA("IfcTriangulatedIrregularNetwork")
}

func isIndexedPolygonalFace(inst *step.Instance) bool {
	return inst.IsA("IfcIndexedPolygonalFace") || inst.IsA("IfcIndexedPolygonalFaceWithVoids")
}

// faceSetMesh tessellates an IfcTriangulatedFaceSet or IfcPolygonalFaceSet, raw
// units. An index out of range, a referenced point that is not three numbers,
// or a face that is not a polygon declines the whole set, because a partial
// mesh is smaller than the element and would under-report its bounds instead of
// falling back to the box.
//
// The one exception to "declined sets are boxed" is a triangulated irregular
// network whose every triangle is flagged invisible: it returns ok with no
// triangles, because the spec excludes a void "without falling back on any
// other geometry".
func faceSetMesh(item *step.Instance) (verts []float32, tris []uint32, ok bool) {
	coords, ok := item.Ref(attrFaceSetCoords)
	if !ok {
		return nil, nil, false
	}
	pts, ok := pointList(coords)
	if !ok {
		return nil, nil, false
	}
	if isTriangulatedFaceSet(item) {
		return triangulatedMesh(item, pts)
	}
	return polygonalMesh(item, pts)
}

func triangulatedMesh(item *step.Instance, pts []step.Value) ([]float32, []uint32, bool) {
	pnV, _ := item.Get(attrTfsPnIndex)
	if isIndexLists(pnV) {
		// The original IFC4 release had NormalIndex, a list of index lists, in
		// the slot ADD2 gave to PnIndex. Normals do not shape the mesh.
		pnV = step.Value{}
	}
	idx, ok := newFaceIndex(pnV, pts)
	if !ok {
		return nil, nil, false
	}
	triV, has := item.Get(attrTfsCoordIndex)
	if !has || triV.Kind != step.KindList || len(triV.List) == 0 {
		return nil, nil, false
	}
	// A network flags each triangle: -2 is an invisible void, -1 an invisible
	// hole, 0-7 record breaklines only. Negative triangles are not part of the
	// surface, so they are validated like the rest but kept out of the mesh.
	var flags []int64
	if item.IsA("IfcTriangulatedIrregularNetwork") {
		flagsV, _ := item.Get(attrTinFlags)
		if flags, ok = intsOf(flagsV); !ok || len(flags) != len(triV.List) {
			return nil, nil, false
		}
	}
	var m faceSetBuilder
	for i, tv := range triV.List {
		corners, ok := intsOf(tv)
		if !ok || len(corners) != 3 {
			return nil, nil, false
		}
		var tri [3]v3
		for j, c := range corners {
			if tri[j], ok = idx.point(c); !ok {
				return nil, nil, false
			}
		}
		if flags != nil && flags[i] < 0 {
			continue
		}
		for _, p := range tri {
			m.tris = append(m.tris, m.vertex(p))
		}
	}
	return m.verts, m.tris, true
}

// polygonalMesh ear-clips each face's outer loop. Inner loops
// (IfcIndexedPolygonalFaceWithVoids) are ignored, which fills the hole — an
// over-report, the same choice brepMesh makes for inner face bounds.
func polygonalMesh(item *step.Instance, pts []step.Value) ([]float32, []uint32, bool) {
	pnV, _ := item.Get(attrPfsPnIndex)
	idx, ok := newFaceIndex(pnV, pts)
	if !ok {
		return nil, nil, false
	}
	facesV, has := item.Get(attrPfsFaces)
	if !has || facesV.Kind != step.KindList || len(facesV.List) == 0 {
		return nil, nil, false
	}
	var verts []float32
	var tris []uint32
	for _, fv := range facesV.List {
		if fv.Kind != step.KindRef || fv.Ref == nil || !isIndexedPolygonalFace(fv.Ref) {
			return nil, nil, false
		}
		loopV, _ := fv.Ref.Get(attrIndexedFaceCoord)
		corners, ok := intsOf(loopV)
		if !ok || len(corners) < 3 {
			return nil, nil, false
		}
		loop := make([]v3, len(corners))
		for i, c := range corners {
			if loop[i], ok = idx.point(c); !ok {
				return nil, nil, false
			}
		}
		base := uint32(len(verts) / 3)
		for _, p := range loop {
			verts = append(verts, float32(p[0]), float32(p[1]), float32(p[2]))
		}
		for _, off := range triangulateFace(loop) {
			tris = append(tris, base+off)
		}
	}
	return verts, tris, len(tris) > 0
}

// faceIndex resolves a face set's 1-based corner indices to points. With a
// PnIndex, corner indices point into it and its values point into the list.
// Points are converted only when a corner references them, so the work is
// bounded by the set's own index list even when many sets share one large
// point list.
type faceIndex struct {
	pts []step.Value
	pn  []int64
}

func newFaceIndex(pnV step.Value, pts []step.Value) (faceIndex, bool) {
	idx := faceIndex{pts: pts}
	if pnV.Kind != step.KindList {
		return idx, true // $: corners index the point list directly
	}
	pn, ok := intsOf(pnV)
	idx.pn = pn
	return idx, ok
}

func (x faceIndex) point(i int64) (v3, bool) {
	if x.pn != nil {
		if i < 1 || i > int64(len(x.pn)) {
			return v3{}, false
		}
		i = x.pn[i-1]
	}
	if i < 1 || i > int64(len(x.pts)) {
		return v3{}, false
	}
	c, ok := numbersOf(x.pts[i-1])
	if !ok || len(c) != 3 {
		return v3{}, false
	}
	return v3{c[0], c[1], c[2]}, true
}

// faceSetBuilder emits only the points triangles reference, so an unused point
// in a shared list cannot inflate the element's bounds.
type faceSetBuilder struct {
	verts []float32
	tris  []uint32
	seen  map[v3]uint32
}

func (m *faceSetBuilder) vertex(p v3) uint32 {
	if i, ok := m.seen[p]; ok {
		return i
	}
	if m.seen == nil {
		m.seen = map[v3]uint32{}
	}
	i := uint32(len(m.verts) / 3)
	m.verts = append(m.verts, float32(p[0]), float32(p[1]), float32(p[2]))
	m.seen[p] = i
	return i
}

// pointList returns an IfcCartesianPointList3D's raw entries; faceIndex
// converts the ones a face references.
func pointList(inst *step.Instance) ([]step.Value, bool) {
	if !inst.IsA("IfcCartesianPointList3D") {
		return nil, false
	}
	listV, has := inst.Get(attrPointListCoords)
	if !has || listV.Kind != step.KindList || len(listV.List) == 0 {
		return nil, false
	}
	return listV.List, true
}

// isIndexLists reports whether v is a non-empty list of lists.
func isIndexLists(v step.Value) bool {
	return v.Kind == step.KindList && len(v.List) > 0 && v.List[0].Kind == step.KindList
}

// numbersOf is strict where floatsOf skips bad members: a coordinate that is
// silently dropped would shift a point rather than decline it.
func numbersOf(v step.Value) ([]float64, bool) {
	if v.Kind != step.KindList {
		return nil, false
	}
	out := make([]float64, len(v.List))
	for i, e := range v.List {
		switch e.Kind {
		case step.KindFloat:
			out[i] = e.F
		case step.KindInt:
			out[i] = float64(e.I)
		default:
			return nil, false
		}
	}
	return out, true
}

func intsOf(v step.Value) ([]int64, bool) {
	if v.Kind != step.KindList {
		return nil, false
	}
	out := make([]int64, len(v.List))
	for i, e := range v.List {
		if e.Kind != step.KindInt {
			return nil, false
		}
		out[i] = e.I
	}
	return out, true
}
