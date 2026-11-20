package geometry

import (
	"errors"
	"fmt"
)

const (
	maxPlanItems    = 10_000  // walls + rooms + slabs + roofs in one plan
	maxPlanElements = 100_000 // elements one plan builds, layers and room walls counted
)

var errThicknessOrLayers = errors.New("give thickness or layers, not both and not neither")

// Plan is a whole plan: walls, rooms, slabs and roofs, each with an ID that names
// its elements. Give Thickness or Layers on an item, not both.
type Plan struct {
	Walls []PlanWall
	Rooms []PlanRoom
	Slabs []PlanSlab
	Roofs []PlanRoof
}

// PlanWall is one wall item.
type PlanWall struct {
	ID                      string
	Edge                    [2][2]float64
	Base, Height, Thickness float64
	Layers                  []float64
	Side                    Side
	Openings                []Opening
	Joins                   []Join // at most one per end
}

// PlanRoom is one room item: a wall on every outline edge.
type PlanRoom struct {
	ID                      string
	Outline                 [][2]float64
	Base, Height, Thickness float64
	Layers                  []float64
	Side                    Side
	Openings                [][]Opening
}

// PlanSlab is one slab item.
type PlanSlab struct {
	ID           string
	Outline      [][2]float64
	Holes        [][][2]float64 // see BuildSlabPolygon
	Z, Thickness float64
	Layers       []float64
}

// PlanRoof is one roof item: a pitched roof over Outline, built as
// BuildRoofLayers does. Gables names the outline edges that are gable ends.
type PlanRoof struct {
	ID              string
	Outline         [][2]float64
	Pitch           float64
	Gables          []int
	Overhang, Eaves float64
	Thickness       float64
	Layers          []float64
}

// Built is what BuildPlan returns: wall items, then room walls, in plan
// order, and the slabs and roofs.
type Built struct {
	Walls []BuiltWall
	Slabs []BuiltSlab
	Roofs []BuiltRoof
}

// BuiltWall is one built wall. Its ID is the item ID (w1), w1/j for layer j,
// r1/i for room wall i, r1/i/j for layer j of it; Wall.Element.GlobalID is
// the same.
type BuiltWall struct {
	ID   string
	Wall Wall
}

// BuiltSlab is one built slab, named like a BuiltWall.
type BuiltSlab struct {
	ID   string
	Slab Slab
}

// BuiltRoof is one built roof, named like a BuiltWall.
type BuiltRoof struct {
	ID   string
	Roof Roof
}

// ItemError is a refusal of one item of a plan.
type ItemError struct {
	Item string
	Err  error
}

func (e *ItemError) Error() string { return e.Item + ": " + e.Err.Error() }
func (e *ItemError) Unwrap() error { return e.Err }

// BuildPlan builds every item of a plan, cuts each joined wall end flush
// with the face it meets, and takes the strip it covers off that face's net
// area. A bad or repeated ID is refused alone, before anything is
// built: one *ItemError per ID. Otherwise it returns every refusal at once
// and no elements: an item's own refusal as an *ItemError, a refused join as
// a *JoinError.
func BuildPlan(p Plan) (Built, error) {
	if n := len(p.Walls) + len(p.Rooms) + len(p.Slabs) + len(p.Roofs); n > maxPlanItems {
		return Built{}, fmt.Errorf("%d items, over the %d limit", n, maxPlanItems)
	}
	if n := planElements(p); n > maxPlanElements {
		return Built{}, fmt.Errorf("the plan builds %d elements, over the %d limit", n, maxPlanElements)
	}
	if err := checkPlanIDs(p); err != nil {
		return Built{}, err
	}

	var (
		out  Built
		errs []error
	)
	addWall := func(id string, w Wall) {
		w.Element.GlobalID = id
		out.Walls = append(out.Walls, BuiltWall{ID: id, Wall: w})
	}
	addSlab := func(id string, s Slab) {
		s.Element.GlobalID = id
		out.Slabs = append(out.Slabs, BuiltSlab{ID: id, Slab: s})
	}
	addRoof := func(id string, r Roof) {
		r.Element.GlobalID = id
		out.Roofs = append(out.Roofs, BuiltRoof{ID: id, Roof: r})
	}
	failed := map[string]bool{}
	wallAt := make([]int, len(p.Walls)) // where each wall item's walls start in out.Walls
	for i, it := range p.Walls {
		wallAt[i] = len(out.Walls)
		if err := buildPlanWall(it, addWall); err != nil {
			errs = append(errs, &ItemError{Item: it.ID, Err: err})
			failed[it.ID] = true
		}
	}
	roomAt := make([]int, len(p.Rooms))
	for i, it := range p.Rooms {
		roomAt[i] = len(out.Walls)
		if err := buildPlanRoom(it, addWall); err != nil {
			errs = append(errs, &ItemError{Item: it.ID, Err: err})
			failed[it.ID] = true
		}
	}
	for _, it := range p.Slabs {
		if err := buildPlanSlab(it, addSlab); err != nil {
			errs = append(errs, &ItemError{Item: it.ID, Err: err})
		}
	}
	for _, it := range p.Roofs {
		if err := buildPlanRoof(it, addRoof); err != nil {
			errs = append(errs, &ItemError{Item: it.ID, Err: err})
		}
	}
	trims, joinErrs := trimJoins(p, failed)
	strips, stripErrs := stripJoins(p, failed, trims)
	errs = append(errs, joinErrs...)
	errs = append(errs, stripErrs...)
	if errs != nil {
		return Built{}, errors.Join(errs...)
	}
	for _, tw := range trims {
		for j, w := range buildTrimmed(p.Walls[tw.item], tw) {
			b := &out.Walls[wallAt[tw.item]+j]
			w.Element.GlobalID = b.ID
			b.Wall = w
		}
	}
	subtractStrips(p, out.Walls, wallAt, roomAt, strips)
	return out, nil
}

// planElements counts the elements a plan builds before any is built: a room
// builds one wall per outline point, and each layer is an element.
func planElements(p Plan) int {
	each := func(layers []float64) int { return max(1, len(layers)) }
	n := 0
	for _, it := range p.Walls {
		n += each(it.Layers)
	}
	for _, it := range p.Rooms {
		n += len(it.Outline) * each(it.Layers)
	}
	for _, it := range p.Slabs {
		n += each(it.Layers)
	}
	for _, it := range p.Roofs {
		n += each(it.Layers)
	}
	return n
}

// validPlanID reports whether id matches ^[A-Za-z0-9_.:-]{1,64}$. It is
// written out because the regexp package adds 127 KB to goifc-build.wasm
// gzipped.
func validPlanID(id string) bool {
	if len(id) == 0 || len(id) > 64 {
		return false
	}
	for _, c := range []byte(id) {
		switch {
		case 'A' <= c && c <= 'Z', 'a' <= c && c <= 'z', '0' <= c && c <= '9', c == '_', c == '.', c == ':', c == '-':
		default:
			return false
		}
	}
	return true
}

// checkPlanIDs refuses an ID that breaks the pattern or is used twice.
func checkPlanIDs(p Plan) error {
	var ids []string
	for _, it := range p.Walls {
		ids = append(ids, it.ID)
	}
	for _, it := range p.Rooms {
		ids = append(ids, it.ID)
	}
	for _, it := range p.Slabs {
		ids = append(ids, it.ID)
	}
	for _, it := range p.Roofs {
		ids = append(ids, it.ID)
	}
	var errs []error
	seen := map[string]bool{}
	for _, id := range ids {
		switch {
		case !validPlanID(id):
			errs = append(errs, &ItemError{Item: id, Err: fmt.Errorf("id %q must be 1 to 64 letters, digits or _ . : -", id)})
		case seen[id]:
			errs = append(errs, &ItemError{Item: id, Err: fmt.Errorf("id %q is used more than once", id)})
		}
		seen[id] = true
	}
	return errors.Join(errs...)
}

func buildPlanWall(it PlanWall, add func(string, Wall)) error {
	if err := checkJoins(it.Joins); err != nil {
		return err
	}
	switch {
	case it.Thickness != 0 && it.Layers == nil:
		w, err := BuildWall(it.Edge, it.Base, it.Height, it.Thickness, it.Side, it.Openings)
		if err != nil {
			return err
		}
		add(it.ID, w)
	case it.Thickness == 0 && it.Layers != nil:
		ws, err := BuildWallLayers(it.Edge, it.Base, it.Height, it.Layers, it.Side, it.Openings)
		if err != nil {
			return err
		}
		for j, w := range ws {
			add(fmt.Sprintf("%s/%d", it.ID, j), w)
		}
	default:
		return errThicknessOrLayers
	}
	return nil
}

func buildPlanRoom(it PlanRoom, add func(string, Wall)) error {
	switch {
	case it.Thickness != 0 && it.Layers == nil:
		ws, err := BuildRoomWalls(it.Outline, it.Base, it.Height, it.Thickness, it.Side, it.Openings)
		if err != nil {
			return err
		}
		for i, w := range ws {
			add(fmt.Sprintf("%s/%d", it.ID, i), w)
		}
	case it.Thickness == 0 && it.Layers != nil:
		ws, err := BuildRoomWallLayers(it.Outline, it.Base, it.Height, it.Layers, it.Side, it.Openings)
		if err != nil {
			return err
		}
		for i, layers := range ws {
			for j, w := range layers {
				add(fmt.Sprintf("%s/%d/%d", it.ID, i, j), w)
			}
		}
	default:
		return errThicknessOrLayers
	}
	return nil
}

func buildPlanSlab(it PlanSlab, add func(string, Slab)) error {
	switch {
	case it.Thickness != 0 && it.Layers == nil:
		s, err := BuildSlabPolygon(Polygon2D{Outer: it.Outline, Holes: it.Holes}, it.Z, it.Thickness)
		if err != nil {
			return err
		}
		add(it.ID, s)
	case it.Thickness == 0 && it.Layers != nil:
		ss, err := BuildSlabPolygonLayers(Polygon2D{Outer: it.Outline, Holes: it.Holes}, it.Z, it.Layers)
		if err != nil {
			return err
		}
		for j, s := range ss {
			add(fmt.Sprintf("%s/%d", it.ID, j), s)
		}
	default:
		return errThicknessOrLayers
	}
	return nil
}

func buildPlanRoof(it PlanRoof, add func(string, Roof)) error {
	spec := RoofSpec{Pitch: it.Pitch, Gables: it.Gables, Overhang: it.Overhang, Eaves: it.Eaves}
	switch {
	case it.Thickness != 0 && it.Layers == nil:
		rs, err := BuildRoofLayers(it.Outline, spec, []float64{it.Thickness})
		if err != nil {
			return err
		}
		add(it.ID, rs[0])
	case it.Thickness == 0 && it.Layers != nil:
		rs, err := BuildRoofLayers(it.Outline, spec, it.Layers)
		if err != nil {
			return err
		}
		for j, r := range rs {
			add(fmt.Sprintf("%s/%d", it.ID, j), r)
		}
	default:
		return errThicknessOrLayers
	}
	return nil
}
