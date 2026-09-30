// Command goifc is the goifc runner for the reader benchmark in parity/bench.
// It times parse, walk and tessellate on one file and prints one JSON record;
// with -boxes it also writes each element's world AABB, outside the timed
// stages, for the agreement check against IfcOpenShell.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/blox-eng/goifc/geometry"
	"github.com/blox-eng/goifc/model"
	"github.com/blox-eng/goifc/step"
)

type record struct {
	Tool       string  `json:"tool"`
	ParseMS    float64 `json:"parse_ms"`
	WalkMS     float64 `json:"walk_ms"`
	GeomMS     float64 `json:"geom_ms"`
	Products   int     `json:"products"`
	Meshes     int     `json:"meshes"`
	PeakRSSMiB float64 `json:"peak_rss_mib"`
}

func main() {
	boxes := flag.String("boxes", "", "write GlobalID -> world AABB JSON here")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: goifc [-boxes out.json] model.ifc")
		os.Exit(2)
	}
	if err := run(flag.Arg(0), *boxes); err != nil {
		fmt.Fprintln(os.Stderr, "goifc:", err)
		os.Exit(1)
	}
}

func run(path, boxesPath string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	rec := record{Tool: "goifc"}

	t := time.Now()
	f, err := step.ParseBytes(src)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	rec.ParseMS = ms(t)

	t = time.Now()
	r, err := model.Extract(f)
	if err != nil {
		return fmt.Errorf("walk: %w", err)
	}
	rec.WalkMS = ms(t)
	rec.Products = len(r.Elements)

	t = time.Now()
	s, err := geometry.Build(f, r)
	if err != nil {
		return fmt.Errorf("tessellate: %w", err)
	}
	rec.GeomMS = ms(t)

	out := map[string]aabb{}
	for i := range s.Elements {
		e := &s.Elements[i]
		if len(e.Verts) == 0 {
			continue
		}
		rec.Meshes++
		out[e.GlobalID] = aabb{Min: e.BBoxMin, Max: e.BBoxMax}
	}
	rec.PeakRSSMiB = peakRSSMiB()

	if boxesPath != "" {
		b, err := json.Marshal(out)
		if err != nil {
			return err
		}
		if err := os.WriteFile(boxesPath, b, 0o644); err != nil {
			return err
		}
	}
	return json.NewEncoder(os.Stdout).Encode(rec)
}

type aabb struct {
	Min [3]float64 `json:"min"`
	Max [3]float64 `json:"max"`
}

func ms(t time.Time) float64 { return float64(time.Since(t).Microseconds()) / 1000 }

// peakRSSMiB reads VmHWM, the same figure the Python and Node runners report.
func peakRSSMiB() float64 {
	b, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(b), "\n") {
		if rest, ok := strings.CutPrefix(line, "VmHWM:"); ok {
			kb, _ := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(rest), " kB"), 64)
			return kb / 1024
		}
	}
	return 0
}
