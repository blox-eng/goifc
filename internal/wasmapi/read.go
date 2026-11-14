package wasmapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"slices"

	ifc "github.com/blox-eng/goifc"
	"github.com/blox-eng/goifc/model"
	"github.com/blox-eng/goifc/step"
)

// readParts are the parts Read can return, in the order an answer lists them.
var readParts = []string{"nodes", "glb", "psets", "storeyPlans", "typeLayers"}

var defaultParts = []string{"nodes", "glb"}

type readAnswer struct {
	GoifcVersion string                                `json:"goifcVersion"`
	OK           bool                                  `json:"ok"`
	Parts        []string                              `json:"parts"`
	Nodes        *[]nodeOut                            `json:"nodes,omitempty"`
	Psets        *map[string]map[string]map[string]any `json:"psets,omitempty"`
	StoreyPlans  *[]storeyPlanOut                      `json:"storeyPlans,omitempty"`
	TypeLayers   *map[string]typeLayersOut             `json:"typeLayers,omitempty"`
	Warnings     []string                              `json:"warnings"`
}

type nodeOut struct {
	GlobalID         string     `json:"globalId"`
	ExpressID        int        `json:"expressId"`
	IFCClass         string     `json:"ifcClass"`
	Name             string     `json:"name"`
	ParentIndex      *int       `json:"parentIndex"`
	Qto              qtoOut     `json:"qto"`
	QuantitySource   string     `json:"quantitySource"`
	OriginMin        [3]float64 `json:"originMin"`
	BBoxMin          [3]float64 `json:"bboxMin"`
	BBoxMax          [3]float64 `json:"bboxMax"`
	Material         string     `json:"material"`
	IsExternal       *bool      `json:"isExternal"`
	NetArea          *float64   `json:"netArea"`
	OpeningPerimeter *float64   `json:"openingPerimeter"`
	OpeningDeduction *float64   `json:"openingDeduction"`
	ProjectedGross   *float64   `json:"projectedGross"`
	HasOpenings      bool       `json:"hasOpenings"`
	TypeGlobalID     string     `json:"typeGlobalId"`
	TypeName         string     `json:"typeName"`
	TypeClass        string     `json:"typeClass"`
}

type qtoOut struct {
	Area      *float64 `json:"area"`
	Volume    *float64 `json:"volume"`
	Length    *float64 `json:"length"`
	Width     *float64 `json:"width"`
	Height    *float64 `json:"height"`
	Perimeter *float64 `json:"perimeter"`
}

type storeyPlanOut struct {
	StoreyGlobalID string            `json:"storeyGlobalId"`
	Elevation      float64           `json:"elevation"`
	Entities       []storeyEntityOut `json:"entities"`
}

type storeyEntityOut struct {
	GlobalID string    `json:"globalId"`
	IFCClass string    `json:"ifcClass"`
	Loops    []loopOut `json:"loops"`
}

type loopOut struct {
	Role   string       `json:"role"`
	Points [][2]float64 `json:"points"`
}

type typeLayersOut struct {
	Layers    []layerOut `json:"layers"`
	Direction string     `json:"direction"`
	Sense     string     `json:"sense"`
}

type layerOut struct {
	MaterialName string   `json:"materialName"`
	ThicknessMm  *float64 `json:"thicknessMm"`
	IsVentilated *bool    `json:"isVentilated"`
	Category     string   `json:"category"`
}

// Read reads one IFC file and returns the JSON answer with the parts in
// include, and the GLB when include lists "glb". A nil include means nodes
// and glb. Any failure refuses the whole answer: no part comes back alone.
// Read never panics on any input.
func Read(version string, ifcBytes []byte, include []string) (answer []byte, glb []byte) {
	if len(ifcBytes) > maxReadBytes {
		return failAnswer(version, []Error{{Code: "too_large", Message: fmt.Sprintf(
			"the file is %s bytes, over the %s-byte cap", thousands(len(ifcBytes)), thousands(maxReadBytes))}}), nil
	}
	parts, err := checkInclude(include)
	if err != nil {
		return failAnswer(version, []Error{{Code: "invalid_input", Message: err.Error()}}), nil
	}
	invalid := func(err error) ([]byte, []byte) {
		return failAnswer(version, []Error{{Code: "invalid_ifc", Message: err.Error()}}), nil
	}
	f, err := step.ParseBytes(ifcBytes)
	if err != nil {
		return invalid(err)
	}
	a, err := ifc.Assemble(f)
	if err != nil {
		return invalid(err)
	}
	im, err := ifc.BuildImportFrom(f, a)
	if err != nil {
		return invalid(err)
	}

	out := readAnswer{GoifcVersion: version, OK: true, Parts: parts, Warnings: []string{}}
	want := func(p string) bool { return slices.Contains(parts, p) }
	if want("nodes") {
		nodes := []nodeOut{}
		for _, n := range im.Nodes {
			nodes = append(nodes, node(n, &out.Warnings))
		}
		out.Nodes = &nodes
	}
	if want("psets") {
		psets := map[string]map[string]map[string]any{}
		for _, e := range a.Result.Elements {
			if e.GlobalID != "" && len(e.Psets) > 0 {
				psets[e.GlobalID] = psetsOut(e.GlobalID, e.Psets, &out.Warnings)
			}
		}
		out.Psets = &psets
	}
	if want("storeyPlans") {
		plans := []storeyPlanOut{}
		for _, sp := range im.StoreyPlans {
			plans = append(plans, storeyPlan(sp))
		}
		out.StoreyPlans = &plans
	}
	if want("typeLayers") {
		layers := map[string]typeLayersOut{}
		for id, tl := range im.TypeLayers {
			layers[id] = typeLayers(tl)
		}
		out.TypeLayers = &layers
	}
	if im.Scene != nil {
		out.Warnings = append(out.Warnings, im.Scene.Warnings...)
	}
	if want("glb") && im.Scene != nil {
		var g bytes.Buffer
		if err := im.Scene.WriteGLB(&g); err != nil {
			return invalid(fmt.Errorf("writing the GLB: %w", err))
		}
		glb = g.Bytes()
	}
	b, err := json.Marshal(out)
	if err != nil {
		return invalid(fmt.Errorf("the file holds a value JSON cannot carry: %w", err))
	}
	return b, glb
}

// checkInclude returns the parts in canonical order, or why include is wrong.
func checkInclude(include []string) ([]string, error) {
	if include == nil {
		return defaultParts, nil
	}
	if len(include) == 0 {
		return nil, fmt.Errorf("include lists no parts: leave it out for the default, or name parts from %q", readParts)
	}
	seen := map[string]bool{}
	for _, p := range include {
		if !slices.Contains(readParts, p) {
			return nil, fmt.Errorf("include names %q, which is not a part: use %q", p, readParts)
		}
		if seen[p] {
			return nil, fmt.Errorf("include lists %q twice", p)
		}
		seen[p] = true
	}
	var parts []string
	for _, p := range readParts {
		if seen[p] {
			parts = append(parts, p)
		}
	}
	return parts, nil
}

func node(n ifc.ImportNode, warnings *[]string) nodeOut {
	fin := func(name string, v *float64) *float64 {
		if v != nil && (math.IsNaN(*v) || math.IsInf(*v, 0)) {
			*warnings = append(*warnings, fmt.Sprintf("%s %s: not a finite number, written as null", n.GlobalID, name))
			return nil
		}
		return v
	}
	return nodeOut{
		GlobalID: n.GlobalID, ExpressID: n.ExpressID, IFCClass: n.IFCClass, Name: n.Name, ParentIndex: n.ParentIndex,
		Qto: qtoOut{
			Area: fin("qto.area", n.Qto.Area), Volume: fin("qto.volume", n.Qto.Volume), Length: fin("qto.length", n.Qto.Length),
			Width: fin("qto.width", n.Qto.Width), Height: fin("qto.height", n.Qto.Height), Perimeter: fin("qto.perimeter", n.Qto.Perimeter),
		},
		QuantitySource: n.QuantitySource, OriginMin: n.OriginMin, BBoxMin: n.BBoxMin, BBoxMax: n.BBoxMax,
		Material: n.Material, IsExternal: n.IsExternal,
		NetArea: fin("netArea", n.NetArea), OpeningPerimeter: fin("openingPerimeter", n.OpeningPerimeter),
		OpeningDeduction: fin("openingDeduction", n.OpeningDeduction), ProjectedGross: fin("projectedGross", n.ProjectedGross),
		HasOpenings: n.HasOpenings, TypeGlobalID: n.TypeGlobalID, TypeName: n.TypeName, TypeClass: n.TypeClass,
	}
}

// psetsOut copies one element's property sets, writing a number JSON cannot
// hold as null and saying so in warnings.
func psetsOut(globalID string, sets map[string]map[string]any, warnings *[]string) map[string]map[string]any {
	out := make(map[string]map[string]any, len(sets))
	for _, set := range sortedKeys(sets) {
		props := make(map[string]any, len(sets[set]))
		for _, prop := range sortedKeys(sets[set]) {
			v := sets[set][prop]
			if x, ok := v.(float64); ok && (math.IsNaN(x) || math.IsInf(x, 0)) {
				*warnings = append(*warnings, fmt.Sprintf("%s %s.%s: not a finite number, written as null", globalID, set, prop))
				v = nil
			}
			props[prop] = v
		}
		out[set] = props
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func storeyPlan(sp ifc.StoreyPlan) storeyPlanOut {
	out := storeyPlanOut{StoreyGlobalID: sp.StoreyGlobalID, Elevation: sp.Elevation, Entities: []storeyEntityOut{}}
	for _, e := range sp.Entities {
		eo := storeyEntityOut{GlobalID: e.GlobalID, IFCClass: e.IFCClass, Loops: []loopOut{}}
		for _, l := range e.Loops {
			pts := l.Points
			if pts == nil {
				pts = [][2]float64{}
			}
			eo.Loops = append(eo.Loops, loopOut{Role: string(l.Role), Points: pts})
		}
		out.Entities = append(out.Entities, eo)
	}
	return out
}

func typeLayers(tl ifc.TypeLayerSet) typeLayersOut {
	out := typeLayersOut{Layers: []layerOut{}, Direction: tl.Direction, Sense: tl.Sense}
	for _, l := range tl.Layers {
		out.Layers = append(out.Layers, layer(l))
	}
	return out
}

func layer(l model.MaterialLayer) layerOut {
	return layerOut{MaterialName: l.MaterialName, ThicknessMm: l.ThicknessMm, IsVentilated: l.IsVentilated, Category: l.Category}
}
