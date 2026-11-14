package wasmapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"

	"github.com/blox-eng/goifc/geometry"
)

// The request. Required numbers are pointers so a missing one is refused
// instead of read as 0; points are [][]float64 so a pair with the wrong
// count is refused instead of padded or cut.
type buildRequest struct {
	Walls   []wallItem `json:"walls"`
	Rooms   []roomItem `json:"rooms"`
	Slabs   []slabItem `json:"slabs"`
	Preview bool       `json:"preview"`
}

type wallItem struct {
	ID        string         `json:"id"`
	Edge      [][]float64    `json:"edge"`
	Base      *float64       `json:"base"`
	Height    *float64       `json:"height"`
	Thickness *float64       `json:"thickness"`
	Layers    []float64      `json:"layers"`
	Side      *string        `json:"side"`
	Openings  []openingInput `json:"openings"`
}

type roomItem struct {
	ID        string           `json:"id"`
	Outline   [][]float64      `json:"outline"`
	Base      *float64         `json:"base"`
	Height    *float64         `json:"height"`
	Thickness *float64         `json:"thickness"`
	Layers    []float64        `json:"layers"`
	Side      *string          `json:"side"`
	Openings  [][]openingInput `json:"openings"`
}

type slabItem struct {
	ID        string      `json:"id"`
	Outline   [][]float64 `json:"outline"`
	Z         *float64    `json:"z"`
	Thickness *float64    `json:"thickness"`
	Layers    []float64   `json:"layers"`
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
	Preview      *previewOut `json:"preview,omitempty"`
}

type wallOut struct {
	ID         string        `json:"id"`
	Quantities wallQuantities `json:"quantities"`
	Measured   faceOut       `json:"measured"`
}

type slabOut struct {
	ID         string         `json:"id"`
	Quantities slabQuantities `json:"quantities"`
	Top        faceOut        `json:"top"`
}

type wallQuantities struct {
	Length          float64 `json:"length"`
	Height          float64 `json:"height"`
	MeasuredGross   float64 `json:"measuredGross"`
	MeasuredNet     float64 `json:"measuredNet"`
	OtherGross      float64 `json:"otherGross"`
	OtherNet        float64 `json:"otherNet"`
	RevealPerimeter float64 `json:"revealPerimeter"`
	GrossVolume     float64 `json:"grossVolume"`
	NetVolume       float64 `json:"netVolume"`
	NicheBack       float64 `json:"nicheBack"`
}

type slabQuantities struct {
	Area      float64 `json:"area"`
	Perimeter float64 `json:"perimeter"`
	Volume    float64 `json:"volume"`
}

type faceOut struct {
	Origin   [3]float64   `json:"origin"`
	U        [3]float64   `json:"u"`
	V        [3]float64   `json:"v"`
	Normal   [3]float64   `json:"normal"`
	Outline  [][2]float64 `json:"outline"`
	Openings []openingOut `json:"openings"`
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

var idPattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,64}$`)

// Build builds every wall, room and slab in one JSON request. It returns the
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
	if n := len(req.Walls) + len(req.Rooms) + len(req.Slabs); n > maxItems {
		return failAnswer(version, []Error{{Code: "too_large", Message: fmt.Sprintf(
			"%s items, over the %s limit", thousands(n), thousands(maxItems))}}), nil
	}
	if errs := checkIDs(req); errs != nil {
		return failAnswer(version, errs), nil
	}

	var (
		errs  []Error
		walls []geometry.Wall
		slabs []geometry.Slab
		out   = built{GoifcVersion: version, OK: true, Walls: []wallOut{}, Slabs: []slabOut{}}
	)
	addWall := func(id string, w geometry.Wall) {
		w.Element.GlobalID = id
		walls = append(walls, w)
		out.Walls = append(out.Walls, wallOut{ID: id, Quantities: wallQuantities(w.Quantities), Measured: face(w.Measured)})
	}
	addSlab := func(id string, s geometry.Slab) {
		s.Element.GlobalID = id
		slabs = append(slabs, s)
		out.Slabs = append(out.Slabs, slabOut{ID: id, Quantities: slabQuantities(s.Quantities), Top: face(s.Top)})
	}
	for _, it := range req.Walls {
		if err := buildWall(it, addWall); err != nil {
			errs = append(errs, toErrors(it.ID, err)...)
		}
	}
	for _, it := range req.Rooms {
		if err := buildRoom(it, addWall); err != nil {
			errs = append(errs, toErrors(it.ID, err)...)
		}
	}
	for _, it := range req.Slabs {
		if err := buildSlab(it, addSlab); err != nil {
			errs = append(errs, toErrors(it.ID, err)...)
		}
	}
	if errs != nil {
		return failAnswer(version, errs), nil
	}

	if req.Preview {
		var svg bytes.Buffer
		if d, err := geometry.WritePreviewSVG(&svg, walls, slabs); err != nil {
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
	return req, nil
}

// checkIDs refuses an id that breaks the pattern or is used twice.
func checkIDs(req buildRequest) []Error {
	var ids []string
	for _, it := range req.Walls {
		ids = append(ids, it.ID)
	}
	for _, it := range req.Rooms {
		ids = append(ids, it.ID)
	}
	for _, it := range req.Slabs {
		ids = append(ids, it.ID)
	}
	var errs []Error
	seen := map[string]bool{}
	for _, id := range ids {
		switch {
		case !idPattern.MatchString(id):
			errs = append(errs, Error{Code: "invalid_input", Item: id, Message: fmt.Sprintf(
				"id %q must be 1 to 64 letters, digits or _ . : -", id)})
		case seen[id]:
			errs = append(errs, Error{Code: "invalid_input", Item: id, Message: fmt.Sprintf(
				"id %q is used more than once", id)})
		}
		seen[id] = true
	}
	return errs
}

func buildWall(it wallItem, add func(string, geometry.Wall)) error {
	pts, err := points("edge", it.Edge)
	if err != nil {
		return err
	}
	if len(pts) != 2 {
		return fmt.Errorf("edge has %d points, want 2", len(pts))
	}
	base, height, side, err := wallCommon(it.Base, it.Height, it.Side)
	if err != nil {
		return err
	}
	ops, err := openings("openings", it.Openings)
	if err != nil {
		return err
	}
	edge := [2][2]float64{pts[0], pts[1]}
	switch {
	case it.Thickness != nil && it.Layers == nil:
		w, err := geometry.BuildWall(edge, base, height, *it.Thickness, side, ops)
		if err != nil {
			return err
		}
		add(it.ID, w)
	case it.Thickness == nil && it.Layers != nil:
		ws, err := geometry.BuildWallLayers(edge, base, height, it.Layers, side, ops)
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

func buildRoom(it roomItem, add func(string, geometry.Wall)) error {
	outline, err := points("outline", it.Outline)
	if err != nil {
		return err
	}
	base, height, side, err := wallCommon(it.Base, it.Height, it.Side)
	if err != nil {
		return err
	}
	var ops [][]geometry.Opening
	if it.Openings != nil {
		ops = make([][]geometry.Opening, len(it.Openings))
		for i, o := range it.Openings {
			if ops[i], err = openings(fmt.Sprintf("openings[%d]", i), o); err != nil {
				return err
			}
		}
	}
	switch {
	case it.Thickness != nil && it.Layers == nil:
		ws, err := geometry.BuildRoomWalls(outline, base, height, *it.Thickness, side, ops)
		if err != nil {
			return err
		}
		for i, w := range ws {
			add(fmt.Sprintf("%s/%d", it.ID, i), w)
		}
	case it.Thickness == nil && it.Layers != nil:
		ws, err := geometry.BuildRoomWallLayers(outline, base, height, it.Layers, side, ops)
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

func buildSlab(it slabItem, add func(string, geometry.Slab)) error {
	outline, err := points("outline", it.Outline)
	if err != nil {
		return err
	}
	if it.Z == nil {
		return errors.New("z is missing")
	}
	switch {
	case it.Thickness != nil && it.Layers == nil:
		s, err := geometry.BuildSlab(outline, *it.Z, *it.Thickness)
		if err != nil {
			return err
		}
		add(it.ID, s)
	case it.Thickness == nil && it.Layers != nil:
		ss, err := geometry.BuildSlabLayers(outline, *it.Z, it.Layers)
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

var errThicknessOrLayers = errors.New("give thickness or layers, not both and not neither")

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

func points(name string, in [][]float64) ([][2]float64, error) {
	if in == nil {
		return nil, fmt.Errorf("%s is missing", name)
	}
	out := make([][2]float64, len(in))
	for i, p := range in {
		if len(p) != 2 {
			return nil, fmt.Errorf("%s point %d has %d numbers, want 2", name, i, len(p))
		}
		out[i] = [2]float64{p[0], p[1]}
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
	out := faceOut{Origin: f.Origin, U: f.U, V: f.V, Normal: f.Normal, Outline: f.Outline, Openings: []openingOut{}}
	if out.Outline == nil {
		out.Outline = [][2]float64{}
	}
	for _, o := range f.Openings {
		out.Openings = append(out.Openings, openingOut(o))
	}
	return out
}
