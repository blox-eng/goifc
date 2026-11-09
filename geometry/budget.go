package geometry

// elementBudget is how much tessellation one element may cost. It counts input
// work (ear tests, loop and face points read, clipped triangles, each mapped
// shape's own cost) and output (every vertex and triangle an item ships), so
// one crafted element can neither stall an import nor clone a shape into
// gigabytes. It bounds each element, not the whole Build (#106). It keeps the
// heaviest corpus element 16 times inside it and the heaviest mapped shape 4
// times inside its share, elementBudget / (maxMapDepth+1), as
// TestCorpusElementsKeepSixteenfoldHeadroom checks. Past it items are boxed or
// left out. A mapped shape costs about twice its own cost at each placement
// (its build, then its clone), so one element holds about
// elementBudget / (2 × shape cost) placements of a shape: measured, about 23
// of the heaviest corpus shape.
const elementBudget = 1 << 23

// budget bounds the work one element's tessellation may do. A nil *budget is
// unlimited, for callers meshing trusted input in isolation; a box walk still
// stops at maxBoxWalk.
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

// reserve charges n units of work about to be done, if they fit. Work that
// does not fit is never done, so it is not charged: the budget is only marked
// spent, and the item may still fall back to its box.
func (b *budget) reserve(n int64) bool {
	if b == nil {
		return true
	}
	if n > b.left {
		b.left = min(b.left, -1)
		return false
	}
	b.left -= n
	return true
}

// spent is how much of the budget has been used.
func (b *budget) spent() int64 { return b.size - b.left }

func (b *budget) exhausted() bool { return b != nil && b.left < 0 }

// canOverdraw reports whether an item may still be meshed or boxed. A box is
// the fallback for work that ran out, so items may overdraw the budget by one
// more budget; past that they are left out.
func (b *budget) canOverdraw() bool { return b == nil || b.left > -b.size }
