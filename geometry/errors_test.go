package geometry

import (
	"bytes"
	"errors"
	"math"
	"testing"

	"github.com/blox-eng/goifc/model"
)

func TestErrorValues(t *testing.T) {
	far := model.Identity()
	far[12] = 3e7
	farWall := Wall{Element: Element{Verts: []float32{0, 0, 0, 1, 0, 0, 0, 1, 0}, Tris: []uint32{0, 1, 2}, Placement: far}}
	farOpening := Wall{Element: Element{Placement: model.Identity()}}
	farOpening.Measured.U = [3]float64{1, 0, 0}
	farOpening.Measured.V = [3]float64{0, 0, 1}
	farOpening.Measured.Openings = []Opening{{Offset: 3e7, Width: 1, Height: 1}}
	preview := func(walls []Wall, slabs []Slab) error {
		_, err := WritePreviewSVG(&bytes.Buffer{}, walls, slabs)
		return err
	}
	tooMany := func(n int, per Element) []Wall {
		walls := make([]Wall, n)
		for i := range walls {
			walls[i].Element = per
		}
		return walls
	}
	boxes := func() error {
		var walls []Wall
		for i := range 10_000 {
			x, y := float64(i%100)*4, float64(i/100)*4
			w, err := BuildWall([2][2]float64{{x, y}, {x + 3, y}}, 0, 2.5, 0.2, SideRight, nil)
			if err != nil {
				t.Fatal(err)
			}
			walls = append(walls, w)
		}
		return preview(walls, nil)
	}
	wall := func(edge [2][2]float64, h float64, ops []Opening) error {
		_, err := BuildWall(edge, 0, h, 0.2, SideLeft, ops)
		return err
	}
	unit := [2][2]float64{{0, 0}, {4, 0}}

	for _, c := range []struct {
		name string
		err  error
		want error
		msg  string
	}{
		{"height over 1,000 m", wall(unit, 1000.5, nil), ErrTooLarge, "height 1000.5 m is over the 1000 m maximum"},
		{"origin over 1e7 m", wall([2][2]float64{{2e7, 0}, {2e7 + 4, 0}}, 2.5, nil), ErrTooLarge, "first point x 2e+07 m is more than 1e+07 m from the origin"},
		{"point over 1,000 m from the first", func() error {
			_, err := BuildSlab([][2]float64{{0, 0}, {1001, 0}, {1001, 4}}, 0, 0.2)
			return err
		}(), ErrTooLarge, "point 1 is more than 1000 m from the first point"},
		{"257 openings", wall(unit, 2.5, make([]Opening, 257)), ErrTooLarge, "wall 0 has 257 openings, more than 256"},
		{"17 layers", func() error {
			_, err := BuildWallLayers(unit, 0, 2.5, make([]float64, 17), SideLeft, nil)
			return err
		}(), ErrTooLarge, "17 layers, more than 16"},
		{"preview elements", preview(tooMany(maxPreviewElements+1, Element{}), nil), ErrTooLarge, "100,001 elements, over the 100,000 limit"},
		{"preview triangles", preview(tooMany(1, Element{Tris: make([]uint32, 3*(maxPreviewTriangles+1))}), nil), ErrTooLarge, "4,000,001 triangles, over the 4,000,000 limit"},
		{"preview vertices", preview(tooMany(1, Element{Verts: make([]float32, 3*(maxPreviewVertices+1))}), nil), ErrTooLarge, "4,000,001 vertices, over the 4,000,000 limit"},
		{"preview vertex far away", preview([]Wall{farWall}, nil), ErrTooLarge, "wall 0: vertex 0 is out of range in the world"},
		{"preview opening far away", preview([]Wall{farOpening}, nil), ErrTooLarge, "wall 0: opening 0 is out of range in the world"},
		{"preview over the byte cap as boxes", boxes(), ErrTooLarge, "preview is 2,960,744 bytes as boxes, over the 1,048,576-byte cap"},
		{"preview of nothing", preview(nil, nil), ErrNothingToDraw, "nothing to draw"},
		{"preview of empty elements", preview([]Wall{{Element: Element{Placement: model.Identity()}}}, nil), ErrNothingToDraw, "nothing to draw"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if !errors.Is(c.err, c.want) {
				t.Fatalf("err %v, want it to wrap %v", c.err, c.want)
			}
			if c.err.Error() != c.msg {
				t.Errorf("message %q, want %q unchanged", c.err.Error(), c.msg)
			}
		})
	}

	slab := func(outline [][2]float64) error {
		_, err := BuildSlab(outline, 0, 0.2)
		return err
	}
	sharp := func() error {
		_, err := BuildRoomWalls([][2]float64{{0, 0}, {10, 0}, {0, 0.1}}, 0, 2.5, 0.2, SideLeft, nil)
		return err
	}
	for _, c := range []struct {
		name string
		err  error
	}{
		{"height under 1 mm", wall(unit, 0.0005, nil)},
		{"height NaN", wall(unit, math.NaN(), nil)},
		{"zero-area outline", slab([][2]float64{{0, 0}, {1, 0}, {2, 0}})},
		{"too-sharp mitre", sharp()},
	} {
		t.Run("not too large: "+c.name, func(t *testing.T) {
			if errors.Is(c.err, ErrTooLarge) || errors.Is(c.err, ErrNothingToDraw) {
				t.Errorf("err %v wraps a limit value, want plain bad input", c.err)
			}
		})
	}
}
