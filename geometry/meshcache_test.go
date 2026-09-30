package geometry

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/blox-eng/goifc/step"
)

func TestMeshCache(t *testing.T) {
	f, err := step.ParseBytes([]byte("ISO-10303-21;\nDATA;\n#1=IFCSHAPEREPRESENTATION($,$,$,());\nENDSEC;\nEND-ISO-10303-21;"))
	if err != nil {
		t.Fatal(err)
	}
	rep, _ := f.ByID(1)
	var builds atomic.Int32
	build := func() mesh {
		builds.Add(1)
		return mesh{tris: []uint32{0, 1, 2}, ok: true}
	}

	var none *meshCache
	none.mapped(rep, 1, 0, build)
	none.mapped(rep, 1, 0, build)
	if n := builds.Load(); n != 2 {
		t.Fatalf("a nil cache built %d times for two calls; want 2", n)
	}

	builds.Store(0)
	c := &meshCache{}
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			if m := c.mapped(rep, 1, 0, build); !m.ok {
				t.Error("cached mesh lost")
			}
		})
	}
	wg.Wait()
	if n := builds.Load(); n != 1 {
		t.Fatalf("16 concurrent calls for one shape built it %d times; want 1", n)
	}
	c.mapped(rep, 0.001, 0, build)
	c.mapped(rep, 1, 1, build)
	if n := builds.Load(); n != 3 {
		t.Fatalf("a new scale and a new depth built %d shapes in all; want 3", n)
	}
}
