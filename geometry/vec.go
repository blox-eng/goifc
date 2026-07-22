package geometry

import (
	"github.com/blox-eng/common/ifc/model"
	"github.com/blox-eng/common/ifc/step"
)

type v3 = [3]float64

// applyMat applies a column-major affine 4x4 (translation at 12,13,14) to p.
func applyMat(m model.Mat4, p v3) v3 {
	return v3{
		m[0]*p[0] + m[4]*p[1] + m[8]*p[2] + m[12],
		m[1]*p[0] + m[5]*p[1] + m[9]*p[2] + m[13],
		m[2]*p[0] + m[6]*p[1] + m[10]*p[2] + m[14],
	}
}

// reverseV3 reverses a point slice in place (used to flip a face loop's winding).
func reverseV3(p []v3) {
	for i, j := 0, len(p)-1; i < j; i, j = i+1, j-1 {
		p[i], p[j] = p[j], p[i]
	}
}

func floatsOf(inst *step.Instance, attr int) []float64 {
	v, ok := inst.Get(attr)
	if !ok || v.Kind != step.KindList {
		return nil
	}
	out := make([]float64, 0, len(v.List))
	for _, e := range v.List {
		switch e.Kind {
		case step.KindFloat:
			out = append(out, e.F)
		case step.KindInt:
			out = append(out, float64(e.I))
		}
	}
	return out
}
