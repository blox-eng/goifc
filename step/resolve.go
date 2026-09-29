package step

import "fmt"

// denseSlack bounds how sparse ids may be before the id index falls back from a
// slice to a map: a slice of up to 4 entries per instance, plus a floor for
// small files, costs less than the map and cannot be blown up by one huge id.
const denseSlack = 4

// finish is parse pass 2, once the slabs have stopped growing: it freezes the
// string arena, decodes the header, and builds the id, type and inverse
// indexes. A reference to a missing instance stays unresolved (Value.Ref is
// nil) and is recorded as a non-fatal warning, mirroring ifcopenshell's SYN 28.
func (p *parser) finish() {
	f := p.f
	f.strs = string(p.strs)
	p.strs, p.stack = nil, nil
	for _, h := range p.header {
		p.setHeader(h.kw, h.args.List())
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

// indexTypes groups instances by type keyword, a complex instance under each of
// its part types. Counting first sizes every group exactly.
func indexTypes(f *File) {
	counts := make(map[string]int)
	each := func(inst *Instance, fn func(string)) {
		if parts := f.complexTypes[inst.id]; len(parts) > 1 {
			for _, pt := range parts {
				fn(pt)
			}
			return
		}
		fn(inst.typ)
	}
	for i := range f.insts {
		each(&f.insts[i], func(t string) { counts[t]++ })
	}
	f.byType = make(map[string][]*Instance, len(counts))
	for t, n := range counts {
		f.byType[t] = make([]*Instance, 0, n)
	}
	for i := range f.insts {
		inst := &f.insts[i]
		each(inst, func(t string) { f.byType[t] = append(f.byType[t], inst) })
	}
}

// indexInverse builds the referrer lists in two passes over every reference —
// count, then fill — so each lands in one flat slice with no per-target
// allocation. Within a target, referrers keep source order and, within one
// referrer, attribute order.
func indexInverse(f *File) {
	counts := make([]uint32, len(f.insts)+1)
	for i := range f.insts {
		inst := &f.insts[i]
		for _, a := range inst.Args() {
			walkRefs(a, func(id uint32) {
				if t := f.index(id); t >= 0 {
					counts[t+1]++
				} else {
					f.warnings = append(f.warnings,
						fmt.Sprintf("instance #%d references missing #%d", inst.id, id))
				}
			})
		}
	}
	for i := 1; i < len(counts); i++ {
		counts[i] += counts[i-1]
	}
	f.invStart = counts
	f.inv = make([]InverseRef, counts[len(counts)-1])
	next := make([]uint32, len(f.insts))
	copy(next, counts[:len(f.insts)])
	for i := range f.insts {
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
