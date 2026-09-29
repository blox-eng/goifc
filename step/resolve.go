package step

import (
	"fmt"
	"runtime"
	"sync"
)

// denseSlack bounds how sparse ids may be before the id index falls back from a
// slice to a map: a slice of up to 4 entries per instance, plus a floor for
// small files, costs less than the map and cannot be blown up by one huge id.
const denseSlack = 4

// finish is parse pass 2, once the parsers are done: it freezes each parser's
// slab, joins their instances in file order, decodes the header, and builds the
// id, type and inverse indexes. A reference to a missing instance stays
// unresolved (Value.Ref is nil) and is recorded as a non-fatal warning,
// mirroring ifcopenshell's SYN 28.
func finish(f *File, ps []*parser) {
	f.slabs = make([]slab, len(ps))
	n := 0
	for _, p := range ps {
		n += len(p.insts)
	}
	f.insts = make([]Instance, 0, n)
	for i, p := range ps {
		f.slabs[i] = slab{vals: p.vals, strs: string(p.strs)}
		f.insts = append(f.insts, p.insts...)
		for id, parts := range p.complex {
			if f.complexTypes == nil {
				f.complexTypes = make(map[uint32][]string)
			}
			f.complexTypes[id] = parts
		}
		p.strs, p.stack, p.insts = nil, nil, nil
	}
	for _, p := range ps {
		for _, h := range p.header {
			p.setHeader(h.kw, h.args.List())
		}
	}
	indexIDs(f)
	indexTypes(f)
	indexInverse(f)
}

// indexIDs builds the id -> instance index. A later instance with a repeated id
// replaces the earlier one, as a map insert would.
func indexIDs(f *File) {
	var maxID uint32
	for i := range f.insts {
		maxID = max(maxID, f.insts[i].id)
	}
	if uint64(maxID) < uint64(denseSlack)*uint64(len(f.insts))+1024 {
		f.dense = make([]int32, maxID+1)
		for i := range f.insts {
			f.dense[f.insts[i].id] = int32(i + 1)
		}
		return
	}
	f.sparse = make(map[uint32]int32, len(f.insts))
	for i := range f.insts {
		f.sparse[f.insts[i].id] = int32(i)
	}
}

// indexGroups is how many instance ranges the type and inverse indexes are
// built over in parallel. Each range holds a count per target instance while
// the inverse index is built, so the count is capped to bound that memory.
const indexGroups = 8

// ranges splits [0, n) into up to g contiguous ranges, in order.
func ranges(n, g int) [][2]int {
	g = max(1, min(g, runtime.GOMAXPROCS(0), n/4096+1))
	out := make([][2]int, g)
	for i := range g {
		out[i] = [2]int{n * i / g, n * (i + 1) / g}
	}
	return out
}

// each runs fn over every range concurrently.
func each(rs [][2]int, fn func(g, lo, hi int)) {
	if len(rs) == 1 {
		fn(0, rs[0][0], rs[0][1])
		return
	}
	var wg sync.WaitGroup
	for g, r := range rs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fn(g, r[0], r[1])
		}()
	}
	wg.Wait()
}

// typesOf calls fn with every type an instance is indexed under: each part of
// a complex instance, or its one type.
func (f *File) typesOf(inst *Instance, fn func(string)) {
	if parts := f.complexTypes[inst.id]; len(parts) > 1 {
		for _, pt := range parts {
			fn(pt)
		}
		return
	}
	fn(inst.typ)
}

// indexTypes groups instances by type keyword, a complex instance under each of
// its part types, in source order. Ranges count their types in parallel; each
// then fills its own stretch of every type's slice.
func indexTypes(f *File) {
	rs := ranges(len(f.insts), indexGroups)
	counts := make([]map[string]int, len(rs))
	each(rs, func(g, lo, hi int) {
		c := make(map[string]int)
		for i := lo; i < hi; i++ {
			f.typesOf(&f.insts[i], func(t string) { c[t]++ })
		}
		counts[g] = c
	})
	f.byType = make(map[string][]*Instance)
	offs := make([]map[string]int, len(rs))
	for g, c := range counts {
		offs[g] = make(map[string]int, len(c))
		for t, n := range c {
			offs[g][t] = len(f.byType[t])
			f.byType[t] = append(f.byType[t], make([]*Instance, n)...)
		}
	}
	each(rs, func(g, lo, hi int) {
		next := offs[g]
		for i := lo; i < hi; i++ {
			inst := &f.insts[i]
			f.typesOf(inst, func(t string) {
				f.byType[t][next[t]] = inst
				next[t]++
			})
		}
	})
}

// indexInverse builds the referrer lists as one flat slice with per-instance
// offsets. Ranges of referrers count their references per target in parallel;
// prefix sums over (target, range) give every range its own stretch of each
// target's list, so the parallel fill keeps source order within a target and,
// within one referrer, attribute order.
func indexInverse(f *File) {
	n := len(f.insts)
	rs := ranges(n, indexGroups)
	counts := make([][]uint32, len(rs))
	warnings := make([][]string, len(rs))
	each(rs, func(g, lo, hi int) {
		c := make([]uint32, n)
		for i := lo; i < hi; i++ {
			inst := &f.insts[i]
			for _, a := range inst.Args() {
				walkRefs(a, func(id uint32) {
					if t := f.index(id); t >= 0 {
						c[t]++
					} else {
						warnings[g] = append(warnings[g],
							fmt.Sprintf("instance #%d references missing #%d", inst.id, id))
					}
				})
			}
		}
		counts[g] = c
	})
	for _, w := range warnings {
		f.warnings = append(f.warnings, w...)
	}
	// Turn each range's counts into its write offsets, target by target.
	f.invStart = make([]uint32, n+1)
	var total uint32
	for t := range n {
		f.invStart[t] = total
		for _, c := range counts {
			k := c[t]
			c[t] = total
			total += k
		}
	}
	f.invStart[n] = total
	f.inv = make([]InverseRef, total)
	each(rs, func(g, lo, hi int) {
		next := counts[g]
		for i := lo; i < hi; i++ {
			inst := &f.insts[i]
			for ai, a := range inst.Args() {
				walkRefs(a, func(id uint32) {
					if t := f.index(id); t >= 0 {
						f.inv[next[t]] = InverseRef{From: inst, AttrIndex: ai}
						next[t]++
					}
				})
			}
		}
	})
}

// walkRefs invokes fn with the target id of every reference within v, recursing
// into lists and typed-value inner args.
func walkRefs(v Value, fn func(uint32)) {
	switch v.Kind {
	case KindRef:
		fn(uint32(v.x))
	case KindList, KindTyped:
		for _, c := range v.List() {
			walkRefs(c, fn)
		}
	}
}
