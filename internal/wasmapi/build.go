package wasmapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"

	"github.com/blox-eng/goifc/geometry"
)

// The request. Required numbers are pointers so a missing one is refused
// instead of read as 0; points are [][]float64 so a pair with the wrong
// count is refused instead of padded or cut.
type buildRequest struct {
	Walls   []wallItem `json:"walls"`
	Rooms   []roomItem `json:"rooms"`
	Slabs   []slabItem `json:"slabs"`
	Roofs   []roofItem `json:"roofs"`
	Preview bool       `json:"preview"`
}

type wallItem struct {
	ID        string         `json:"id"`
	Edge      [][]*float64   `json:"edge"`
	Base      *float64       `json:"base"`
	Height    *float64       `json:"height"`
	Thickness *float64       `json:"thickness"`
	Layers    []*float64     `json:"layers"`
	Side      *string        `json:"side"`
	Openings  []openingInput `json:"openings"`
	Joins     []joinInput    `json:"joins"`
}

type joinInput struct {
	End  *string `json:"end"`
	Wall *string `json:"wall"`
}

type roomItem struct {
	ID        string           `json:"id"`
	Outline   [][]*float64     `json:"outline"`
	Base      *float64         `json:"base"`
	Height    *float64         `json:"height"`
	Thickness *float64         `json:"thickness"`
	Layers    []*float64       `json:"layers"`
	Side      *string          `json:"side"`
	Openings  [][]openingInput `json:"openings"`
}

type slabItem struct {
	ID        string         `json:"id"`
	Outline   [][]*float64   `json:"outline"`
	Holes     [][][]*float64 `json:"holes"`
	Z         *float64       `json:"z"`
	Thickness *float64       `json:"thickness"`
	Layers    []*float64     `json:"layers"`
}

type roofItem struct {
	ID        string       `json:"id"`
	Outline   [][]*float64 `json:"outline"`
	Pitch     *float64     `json:"pitch"`
	Gables    []*int       `json:"gables"`
	Overhang  float64      `json:"overhang"`
	Eaves     *float64     `json:"eaves"`
	Thickness *float64     `json:"thickness"`
	Layers    []*float64   `json:"layers"`
}

type openingInput struct {
	Offset *float64 `json:"offset"`
	Sill   *float64 `json:"sill"`
	Width  *float64 `json:"width"`
	Height *float64 `json:"height"`
	Depth  float64  `json:"depth"`
}

// The answer.
type built struct {
	GoifcVersion string      `json:"goifcVersion"`
	OK           bool        `json:"ok"`
	Walls        []wallOut   `json:"walls"`
	Slabs        []slabOut   `json:"slabs"`
	Roofs        []roofOut   `json:"roofs,omitempty"`
	Preview      *previewOut `json:"preview,omitempty"`
}

type wallOut struct {
	ID         string         `json:"id"`
	Quantities wallQuantities `json:"quantities"`
	Measured   faceOut        `json:"measured"`
}

type slabOut struct {
	ID         string         `json:"id"`
	Quantities slabQuantities `json:"quantities"`
	Top        faceOut        `json:"top"`
}

type roofOut struct {
	ID         string         `json:"id"`
	Quantities roofQuantities `json:"quantities"`
	Planes     []faceOut      `json:"planes"`
}

type roofQuantities struct {
	SlopeArea float64 `json:"slopeArea"`
	PlanArea  float64 `json:"planArea"`
	Volume    float64 `json:"volume"`
	Eaves     float64 `json:"eaves"`
	Ridge     float64 `json:"ridge"`
	Hip       float64 `json:"hip"`
	Valley    float64 `json:"valley"`
	Verge     float64 `json:"verge"`
	Top       float64 `json:"top"`
}

type wallQuantities struct {
	Length          float64 `json:"length"`
	Height          float64 `json:"height"`
	MeasuredGross   float64 `json:"measuredGross"`
	MeasuredJoins   float64 `json:"measuredJoins"`
	MeasuredNet     float64 `json:"measuredNet"`
	OtherGross      float64 `json:"otherGross"`
	OtherJoins      float64 `json:"otherJoins"`
	OtherNet        float64 `json:"otherNet"`
	RevealPerimeter float64 `json:"revealPerimeter"`
	GrossVolume     float64 `json:"grossVolume"`
	NetVolume       float64 `json:"netVolume"`
	NicheBack       float64 `json:"nicheBack"`

	MeasuredFloorLine float64 `json:"measuredFloorLine"`
	OtherFloorLine    float64 `json:"otherFloorLine"`
}

type slabQuantities struct {
	Area          float64 `json:"area"`
	Perimeter     float64 `json:"perimeter"`
	Volume        float64 `json:"volume"`
	HolePerimeter float64 `json:"holePerimeter"`
}

type faceOut struct {
	Origin   [3]float64     `json:"origin"`
	U        [3]float64     `json:"u"`
	V        [3]float64     `json:"v"`
	Normal   [3]float64     `json:"normal"`
	Outline  [][2]float64   `json:"outline"`
	Openings []openingOut   `json:"openings"`
	Holes    [][][2]float64 `json:"holes,omitempty"`
}

type openingOut struct {
	Offset float64 `json:"offset"`
	Sill   float64 `json:"sill"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
	Depth  float64 `json:"depth"`
}

type previewOut struct {
	Detail string `json:"detail,omitempty"`
	SVG    string `json:"svg,omitempty"`
	Error  *Error `json:"error,omitempty"`
}

// Build builds every wall, room, slab and roof in one JSON request. It returns the
// JSON answer and, when the answer is ok, a GLB with one node per built
// element, named by its element id. A refused request returns every refusal
// at once and no GLB. Build never panics on any input.
func Build(version string, request []byte) (answer []byte, glb []byte) {
	if len(request) > maxRequestBytes {
		return failAnswer(version, []Error{{Code: "too_large", Message: fmt.Sprintf(
			"the request is %s bytes, over the %s-byte cap", thousands(len(request)), thousands(maxRequestBytes))}}), nil
	}
	req, err := decodeRequest(request)
	if err != nil {
		return failAnswer(version, []Error{{Code: "invalid_input", Message: err.Error()}}), nil
	}
	if n := len(req.Walls) + len(req.Rooms) + len(req.Slabs) + len(req.Roofs); n > maxItems {
		return failAnswer(version, []Error{{Code: "too_large", Message: fmt.Sprintf(
			"%s items, over the %s limit", thousands(n), thousands(maxItems))}}), nil
	}
	if n := countElements(req); n > maxElements {
		return failAnswer(version, []Error{{Code: "too_large", Message: fmt.Sprintf(
			"the request builds %s elements, over the %s limit", thousands(n), thousands(maxElements))}}), nil
	}
	plan, errs := toPlan(req)
	if errs != nil {
		return failAnswer(version, errs), nil
	}
	b, err := geometry.BuildPlan(plan)
	if err != nil {
		return failAnswer(version, toErrors(err)), nil
	}

	var (
		walls []geometry.Wall
		slabs []geometry.Slab
		roofs []geometry.Roof
		out   = built{GoifcVersion: version, OK: true, Walls: []wallOut{}, Slabs: []slabOut{}}
	)
	for _, w := range b.Walls {
		walls = append(walls, w.Wall)
		out.Walls = append(out.Walls, wallOut{ID: w.ID, Quantities: toWallQuantities(w.Wall.Quantities), Measured: face(w.Wall.Measured)})
	}
	for _, s := range b.Slabs {
		slabs = append(slabs, s.Slab)
		q := s.Slab.Quantities
		out.Slabs = append(out.Slabs, slabOut{ID: s.ID, Quantities: slabQuantities{Area: q.Area, Perimeter: q.Perimeter, Volume: q.Volume, HolePerimeter: q.HolePerimeter}, Top: face(s.Slab.Top)})
	}
	for _, r := range b.Roofs {
		roofs = append(roofs, r.Roof)
		out.Roofs = append(out.Roofs, toRoofOut(r))
	}

	if req.Preview {
		var svg bytes.Buffer
		drawn := slabs[:len(slabs):len(slabs)]
		for _, r := range roofs {
			drawn = append(drawn, geometry.Slab{Element: r.Element})
		}
		if d, err := geometry.WritePreviewSVG(&svg, walls, drawn); err != nil {
			e := Error{Code: codeOf(err), Message: err.Error()}
			out.Preview = &previewOut{Error: &e}
		} else {
			out.Preview = &previewOut{Detail: string(d), SVG: svg.String()}
		}
	}

	scene := geometry.Scene{}
	for _, w := range walls {
		scene.Elements = append(scene.Elements, w.Element)
	}
	for _, s := range slabs {
		scene.Elements = append(scene.Elements, s.Element)
	}
	for _, r := range roofs {
		scene.Elements = append(scene.Elements, r.Element)
	}
	var g bytes.Buffer
	if err := scene.WriteGLB(&g); err != nil {
		return failAnswer(version, []Error{{Code: "invalid_input", Message: "writing the GLB: " + err.Error()}}), nil
	}
	return mustJSON(out), g.Bytes()
}

// decodeRequest reads exactly one JSON object with no unknown fields.
func decodeRequest(request []byte) (buildRequest, error) {
	var req buildRequest
	if t := bytes.TrimLeft(request, " \t\r\n"); len(t) == 0 || t[0] != '{' {
		return req, errors.New("the request must be one JSON object")
	}
	dec := json.NewDecoder(bytes.NewReader(request))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		return req, fmt.Errorf("the request is not valid: %w", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return req, errors.New("the request has more data after the request object")
	}
	if err := checkKeys(request, reflect.TypeOf(req), ""); err != nil {
		return req, fmt.Errorf("the request is not valid: %w", err)
	}
	return req, nil
}

// countElements counts the elements a request builds before any is built:
// a room builds one wall per outline point, and each layer is an element.
func countElements(req buildRequest) int {
	each := func(layers []*float64) int { return max(1, len(layers)) }
	n := 0
	for _, it := range req.Walls {
		n += each(it.Layers)
	}
	for _, it := range req.Rooms {
		n += len(it.Outline) * each(it.Layers)
	}
	for _, it := range req.Slabs {
		n += each(it.Layers)
	}
	for _, it := range req.Roofs {
		n += each(it.Layers)
	}
	return n
}

// toPlan reads the request's numbers into a Plan, refusing a missing or null
// one. It returns every refusal at once.
func toPlan(req buildRequest) (geometry.Plan, []Error) {
	var (
		p    geometry.Plan
		errs []Error
	)
	refuse := func(id string, err error) {
		errs = append(errs, Error{Code: "invalid_input", Item: id, Message: err.Error()})
	}
	for _, it := range req.Walls {
		w, err := toPlanWall(it)
		if err != nil {
			refuse(it.ID, err)
			continue
		}
		p.Walls = append(p.Walls, w)
	}
	for _, it := range req.Rooms {
		r, err := toPlanRoom(it)
		if err != nil {
			refuse(it.ID, err)
			continue
		}
		p.Rooms = append(p.Rooms, r)
	}
	for _, it := range req.Slabs {
		s, err := toPlanSlab(it)
		if err != nil {
			refuse(it.ID, err)
			continue
		}
		p.Slabs = append(p.Slabs, s)
	}
	for _, it := range req.Roofs {
		r, err := toPlanRoof(it)
		if err != nil {
			refuse(it.ID, err)
			continue
		}
		p.Roofs = append(p.Roofs, r)
	}
	return p, errs
}

func toPlanWall(it wallItem) (geometry.PlanWall, error) {
	w := geometry.PlanWall{ID: it.ID}
	pts, err := points("edge", it.Edge)
	if err != nil {
		return w, err
	}
	if len(pts) != 2 {
		return w, fmt.Errorf("edge has %d points, want 2", len(pts))
	}
	w.Edge = [2][2]float64{pts[0], pts[1]}
	if w.Base, w.Height, w.Side, err = wallCommon(it.Base, it.Height, it.Side); err != nil {
		return w, err
	}
	if w.Openings, err = openings("openings", it.Openings); err != nil {
		return w, err
	}
	if w.Joins, err = joins(it.Joins); err != nil {
		return w, err
	}
	w.Thickness, w.Layers, err = thicknessOrLayers(it.Thickness, it.Layers)
	return w, err
}

func toPlanRoom(it roomItem) (geometry.PlanRoom, error) {
	r := geometry.PlanRoom{ID: it.ID}
	var err error
	if r.Outline, err = points("outline", it.Outline); err != nil {
		return r, err
	}
	if r.Base, r.Height, r.Side, err = wallCommon(it.Base, it.Height, it.Side); err != nil {
		return r, err
	}
	if it.Openings != nil {
		r.Openings = make([][]geometry.Opening, len(it.Openings))
		for i, o := range it.Openings {
			if r.Openings[i], err = openings(fmt.Sprintf("openings[%d]", i), o); err != nil {
				return r, err
			}
		}
	}
	r.Thickness, r.Layers, err = thicknessOrLayers(it.Thickness, it.Layers)
	return r, err
}

func toPlanSlab(it slabItem) (geometry.PlanSlab, error) {
	s := geometry.PlanSlab{ID: it.ID}
	var err error
	if s.Outline, err = points("outline", it.Outline); err != nil {
		return s, err
	}
	for i, h := range it.Holes {
		ring, err := points(fmt.Sprintf("holes[%d]", i), h)
		if err != nil {
			return s, err
		}
		s.Holes = append(s.Holes, ring)
	}
	if it.Z == nil {
		return s, errors.New("z is missing")
	}
	s.Z = *it.Z
	s.Thickness, s.Layers, err = thicknessOrLayers(it.Thickness, it.Layers)
	return s, err
}

func toPlanRoof(it roofItem) (geometry.PlanRoof, error) {
	r := geometry.PlanRoof{ID: it.ID, Overhang: it.Overhang}
	var err error
	if r.Outline, err = points("outline", it.Outline); err != nil {
		return r, err
	}
	switch {
	case it.Pitch == nil:
		return r, errors.New("pitch is missing")
	case it.Eaves == nil:
		return r, errors.New("eaves is missing")
	}
	r.Pitch, r.Eaves = *it.Pitch, *it.Eaves
	for i, g := range it.Gables {
		if g == nil {
			return r, fmt.Errorf("gables[%d] is null, want an edge number", i)
		}
		r.Gables = append(r.Gables, *g)
	}
	r.Thickness, r.Layers, err = thicknessOrLayers(it.Thickness, it.Layers)
	return r, err
}

func thicknessOrLayers(thickness *float64, layers []*float64) (float64, []float64, error) {
	switch {
	case thickness != nil && layers == nil:
		if *thickness == 0 { // a Plan reads 0 as "not given"
			return 0, nil, errors.New("thickness 0 m is under the 1 mm minimum")
		}
		return *thickness, nil, nil
	case thickness == nil && layers != nil:
		l, err := numbers("layers", layers)
		return 0, l, err
	}
	return 0, nil, errors.New("give thickness or layers, not both and not neither")
}

func joins(in []joinInput) ([]geometry.Join, error) {
	var out []geometry.Join
	for i, j := range in {
		if j.End == nil {
			return nil, fmt.Errorf("joins[%d]: end is missing", i)
		}
		if j.Wall == nil {
			return nil, fmt.Errorf("joins[%d]: wall is missing", i)
		}
		var end geometry.End
		if err := end.UnmarshalText([]byte(*j.End)); err != nil {
			return nil, fmt.Errorf(`joins[%d]: end %q: give "a" or "b"`, i, *j.End)
		}
		out = append(out, geometry.Join{End: end, Wall: *j.Wall})
	}
	return out, nil
}

func wallCommon(base, height *float64, side *string) (float64, float64, geometry.Side, error) {
	var s geometry.Side
	switch {
	case base == nil:
		return 0, 0, s, errors.New("base is missing")
	case height == nil:
		return 0, 0, s, errors.New("height is missing")
	case side == nil:
		return 0, 0, s, errors.New(`side is missing: give "left" or "right"`)
	}
	if err := s.UnmarshalText([]byte(*side)); err != nil {
		return 0, 0, s, fmt.Errorf(`side %q: give "left" or "right"`, *side)
	}
	return *base, *height, s, nil
}

func points(name string, in [][]*float64) ([][2]float64, error) {
	if in == nil {
		return nil, fmt.Errorf("%s is missing", name)
	}
	out := make([][2]float64, len(in))
	for i, p := range in {
		if len(p) != 2 {
			return nil, fmt.Errorf("%s point %d has %d numbers, want 2", name, i, len(p))
		}
		if p[0] == nil || p[1] == nil {
			return nil, fmt.Errorf("%s point %d has a null, want 2 numbers", name, i)
		}
		out[i] = [2]float64{*p[0], *p[1]}
	}
	return out, nil
}

// numbers refuses a null where a list wants a number, so it never reads as 0.
func numbers(name string, in []*float64) ([]float64, error) {
	out := make([]float64, len(in))
	for i, v := range in {
		if v == nil {
			return nil, fmt.Errorf("%s[%d] is null, want a number", name, i)
		}
		out[i] = *v
	}
	return out, nil
}

func openings(name string, in []openingInput) ([]geometry.Opening, error) {
	var out []geometry.Opening
	for i, o := range in {
		for _, f := range [...]struct {
			field string
			v     *float64
		}{{"offset", o.Offset}, {"sill", o.Sill}, {"width", o.Width}, {"height", o.Height}} {
			if f.v == nil {
				return nil, fmt.Errorf("%s[%d]: %s is missing", name, i, f.field)
			}
		}
		out = append(out, geometry.Opening{Offset: *o.Offset, Sill: *o.Sill, Width: *o.Width, Height: *o.Height, Depth: o.Depth})
	}
	return out, nil
}

func face(f geometry.Face) faceOut {
	out := faceOut{Origin: f.Origin, U: f.U, V: f.V, Normal: f.Normal, Outline: f.Outline, Openings: []openingOut{}, Holes: f.Holes}
	if out.Outline == nil {
		out.Outline = [][2]float64{}
	}
	for _, o := range f.Openings {
		out.Openings = append(out.Openings, openingOut(o))
	}
	return out
}

func toRoofOut(b geometry.BuiltRoof) roofOut {
	q := b.Roof.Quantities
	out := roofOut{ID: b.ID, Planes: []faceOut{}, Quantities: roofQuantities{
		SlopeArea: q.SlopeArea, PlanArea: q.PlanArea, Volume: q.Volume,
		Eaves: q.Eaves, Ridge: q.Ridge, Hip: q.Hip, Valley: q.Valley, Verge: q.Verge, Top: q.Top,
	}}
	for _, p := range b.Roof.Planes {
		out.Planes = append(out.Planes, face(p))
	}
	return out
}

func toWallQuantities(q geometry.WallQuantities) wallQuantities {
	return wallQuantities{
		Length: q.Length, Height: q.Height,
		MeasuredGross: q.MeasuredGross, MeasuredJoins: q.MeasuredJoins, MeasuredNet: q.MeasuredNet,
		OtherGross: q.OtherGross, OtherJoins: q.OtherJoins, OtherNet: q.OtherNet,
		RevealPerimeter: q.RevealPerimeter,
		GrossVolume:     q.GrossVolume, NetVolume: q.NetVolume,
		NicheBack:         q.NicheBack,
		MeasuredFloorLine: q.MeasuredFloorLine,
		OtherFloorLine:    q.OtherFloorLine,
	}
}
