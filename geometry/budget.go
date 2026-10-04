package geometry

// elementBudget is how much tessellation work one element may cost, counted in
// units of input work: ear tests, loop and face points read, clipped triangles,
// and each mapped shape's own cost. It bounds work, not output size. The
// heaviest element in the corpus costs a small fraction of it; a crafted file
// (one huge face referenced thousands of times, a comb-shaped loop) reaches it
// in about half a second, and past it items are boxed or left out.
const elementBudget = 1 << 26

// budget bounds the work one element's tessellation may do. A nil *budget is
// unlimited, for callers meshing trusted input in isolation.
type budget struct{ left, size int64 }

func newBudget() *budget { return sizedBudget(elementBudget) }

func sizedBudget(n int64) *budget { return &budget{left: n, size: n} }

// spend charges n units and reports whether the budget still holds.
func (b *budget) spend(n int64) bool {
	if b == nil {
		return true
	}
	b.left -= n
	return b.left >= 0
}

// spent is how much of the budget has been used.
func (b *budget) spent() int64 { return b.size - b.left }

func (b *budget) exhausted() bool { return b != nil && b.left < 0 }
