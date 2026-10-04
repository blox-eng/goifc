package geometry

// elementBudget is how much tessellation work one element may cost, counted in
// ear-clip steps and points converted. The heaviest element in the corpus costs
// a small fraction of it; a crafted file (one huge face referenced thousands of
// times, a comb-shaped loop) reaches it in well under a second and is boxed.
const elementBudget = 1 << 26

// budget bounds the work one element's tessellation may do. A nil *budget is
// unlimited, for callers meshing trusted input in isolation.
type budget struct{ left int64 }

func newBudget() *budget { return &budget{left: elementBudget} }

// spend charges n units and reports whether the budget still holds.
func (b *budget) spend(n int) bool {
	if b == nil {
		return true
	}
	b.left -= int64(n)
	return b.left >= 0
}

func (b *budget) exhausted() bool { return b != nil && b.left < 0 }
