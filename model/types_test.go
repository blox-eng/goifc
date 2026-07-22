package model

import (
	"math"
	"testing"
)

func TestMat4IdentityAndMul(t *testing.T) {
	id := Identity()
	// identity * identity = identity
	got := id.Mul(id)
	if got != id {
		t.Fatalf("id*id = %v, want identity", got)
	}
	// A translation composed onto identity yields that translation.
	trans := Identity()
	trans[12], trans[13], trans[14] = 2, 3, 4 // column-major: last column is translation
	out := id.Mul(trans)
	x, y, z := out.Translation()
	if x != 2 || y != 3 || z != 4 {
		t.Fatalf("translation = %v,%v,%v want 2,3,4", x, y, z)
	}
}

func TestMat4MulComposesRotationThenTranslation(t *testing.T) {
	// 90° about Z: X axis -> +Y. Column-major basis columns.
	rot := Identity()
	rot[0], rot[1] = 0, 1 // new X column = (0,1,0)
	rot[4], rot[5] = -1, 0 // new Y column = (-1,0,0)
	trans := Identity()
	trans[12] = 5 // translate +5 X in parent frame
	world := trans.Mul(rot) // parent(trans) applied to child(rot)
	x, _, _ := world.Translation()
	if math.Abs(x-5) > 1e-9 {
		t.Fatalf("origin x = %v want 5", x)
	}
}
