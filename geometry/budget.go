package geometry

// elementBudget is how much tessellation one element may cost. It counts input
// work (ear tests, loop and face points read, clipped triangles, each mapped
// shape's own cost) and output (every vertex and triangle an item ships), so a
// crafted file can neither stall an import nor clone a shape into gigabytes.
// It keeps the heaviest corpus element 16 times inside it and the heaviest
// mapped shape 4 times inside its ninth, as
// TestCorpusElementsKeepSixteenfoldHeadroom checks. Past it items are boxed or
// left out.
const elementBudget = 1 << 23

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

// canOverdraw reports whether an item may still be meshed or boxed. A box is
// the fallback for work that ran out, so items may overdraw the budget by one
// more budget; past that they are left out.
func (b *budget) canOverdraw() bool { return b == nil || b.left > -b.size }
