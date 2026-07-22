package step

import "fmt"

// resolveAndIndex is parse pass 2. For every instance attribute it resolves each
// #ref to its target *Instance (in place) and records the reverse edge in the
// inverse index, keyed by the referenced id, tagged with the referrer's top-level
// attribute index. A reference to a missing instance is left unresolved (Ref nil)
// and recorded as a non-fatal warning, mirroring ifcopenshell's SYN 28.
func resolveAndIndex(f *File) {
	for _, id := range f.order {
		inst := f.byID[id]
		for ai := range inst.args {
			// walkRefs runs synchronously, so capturing the loop var ai directly is
			// safe. args[ai:ai+1] scopes the walk to one top-level attribute so its
			// index can be recorded on every ref nested within it.
			walkRefs(inst.args[ai:ai+1], func(v *Value) {
				if target, ok := f.byID[v.RefID]; ok {
					v.Ref = target
					f.inverse[v.RefID] = append(f.inverse[v.RefID], InverseRef{From: inst, AttrIndex: ai})
				} else {
					f.warnings = append(f.warnings,
						fmt.Sprintf("instance #%d references missing #%d", inst.id, v.RefID))
				}
			})
		}
	}
}

// walkRefs invokes fn on every KindRef value within vs, recursing into lists and
// typed-value inner args. It takes the slice by reference and addresses elements
// with &vs[i] so fn's writes to v.Ref persist in the stored attribute tree.
func walkRefs(vs []Value, fn func(*Value)) {
	for i := range vs {
		if vs[i].Kind == KindRef {
			fn(&vs[i])
		}
		if len(vs[i].List) > 0 {
			walkRefs(vs[i].List, fn)
		}
	}
}
