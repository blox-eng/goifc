package geometry

import (
	"io"
	"math"

	"github.com/blox-eng/common/ifc/model"
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
		positions := make([][3]float32, len(e.Verts)/3)
		for k := range positions {
			positions[k] = [3]float32{e.Verts[3*k], e.Verts[3*k+1], e.Verts[3*k+2]}
		}
		posAcc := modeler.WritePosition(doc, positions)
		idxAcc := modeler.WriteIndices(doc, e.Tris)
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

// gltfMatrix converts a model.Mat4 (column-major, translation at 12,13,14) to a
// gltf node matrix ([16]float64, same column-major glTF layout).
func gltfMatrix(m model.Mat4) [16]float64 {
	var out [16]float64
	copy(out[:], m[:])
	return out
}
