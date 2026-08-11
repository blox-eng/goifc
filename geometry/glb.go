package geometry

import (
	"io"
	"math"

	"github.com/blox-eng/goifc/model"
	"github.com/qmuntal/gltf"
	"github.com/qmuntal/gltf/modeler"
)

// WriteGLB emits one binary glTF. A single root node applies a Z-up->Y-up
// rotation (Rx(-90 deg)); every element is a child node named by its GlobalId,
// with node.Matrix = element Placement (world, meters, Z-up) and a mesh of the
// element-local verts/tris. All meshes share one binary buffer.
func (s *Scene) WriteGLB(w io.Writer) error {
	doc := gltf.NewDocument()

	root := &gltf.Node{Name: "Z_UP_TO_Y_UP"}
	// Rx(-90 deg) as a quaternion (x=-sin45, w=cos45): rotates +Z -> +Y.
	root.Rotation = [4]float64{-math.Sqrt2 / 2, 0, 0, math.Sqrt2 / 2}
	doc.Nodes = append(doc.Nodes, root)
	rootIdx := len(doc.Nodes) - 1
	doc.Scenes[0].Nodes = append(doc.Scenes[0].Nodes, rootIdx)

	for i := range s.Elements {
		e := &s.Elements[i]
		if len(e.Verts) == 0 || len(e.Tris) == 0 {
			continue
		}
		// Weld coincident vertices: the brep mesher emits a fresh vertex block per
		// IfcFace (no cross-face sharing), which bloats the GLB ~4x on real files
		// (kb645: 2.39M → 592K verts). Welding by quantized local position restores
		// vertex sharing — Go-native, no meshopt/gltfpack dependency (the epic's
		// "tiny meshes, no optimizer" thesis holds once verts are actually shared).
		positions, indices := weldMesh(e.Verts, e.Tris)
		posAcc := modeler.WritePosition(doc, positions)
		idxAcc := modeler.WriteIndices(doc, indices)
		mesh := &gltf.Mesh{Primitives: []*gltf.Primitive{{
			Indices:    gltf.Index(idxAcc),
			Attributes: gltf.PrimitiveAttributes{gltf.POSITION: posAcc},
		}}}
		doc.Meshes = append(doc.Meshes, mesh)
		meshIdx := len(doc.Meshes) - 1

		node := &gltf.Node{Name: e.GlobalID, Mesh: gltf.Index(meshIdx)}
		node.Matrix = gltfMatrix(e.Placement)
		doc.Nodes = append(doc.Nodes, node)
		childIdx := len(doc.Nodes) - 1
		root.Children = append(root.Children, childIdx)
	}

	enc := gltf.NewEncoder(w)
	enc.AsBinary = true
	return enc.Encode(doc)
}

// weldMesh deduplicates coincident vertices (quantized to 1e-5 m in element-LOCAL
// space, where coords are element-sized so the quantum is safe) and returns the
// unique position list plus indices remapped onto it. Triangles are preserved
// exactly; only redundant vertex copies are removed.
func weldMesh(verts []float32, tris []uint32) ([][3]float32, []uint32) {
	const q = 1e5
	uniq := make(map[[3]int64]uint32)
	positions := make([][3]float32, 0, len(verts)/3)
	remap := make([]uint32, len(verts)/3)
	for k := range remap {
		p := [3]float32{verts[3*k], verts[3*k+1], verts[3*k+2]}
		key := [3]int64{
			int64(math.Round(float64(p[0]) * q)),
			int64(math.Round(float64(p[1]) * q)),
			int64(math.Round(float64(p[2]) * q)),
		}
		id, ok := uniq[key]
		if !ok {
			id = uint32(len(positions))
			uniq[key] = id
			positions = append(positions, p)
		}
		remap[k] = id
	}
	indices := make([]uint32, 0, len(tris))
	for i := 0; i+2 < len(tris); i += 3 {
		a, b, c := tris[i], tris[i+1], tris[i+2]
		if int(a) >= len(remap) || int(b) >= len(remap) || int(c) >= len(remap) {
			// Malformed triangle (mesher shouldn't emit this) — drop it whole
			// rather than remap to vertex 0, which would produce a corrupt tri.
			continue
		}
		indices = append(indices, remap[a], remap[b], remap[c])
	}
	return positions, indices
}

// gltfMatrix converts a model.Mat4 (column-major, translation at 12,13,14) to a
// gltf node matrix ([16]float64, same column-major glTF layout).
func gltfMatrix(m model.Mat4) [16]float64 {
	var out [16]float64
	copy(out[:], m[:])
	return out
}
