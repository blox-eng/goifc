// Package step is a schema-agnostic STEP/SPF (ISO 10303-21) tokenizer and entity
// graph for IFC files, ported from ifcopenshell's parser and entity_instance model
// into idiomatic Go. It parses a .ifc file in-process into a navigable graph of
// entity instances with forward AND inverse references, using no CAD kernel, no
// Python, and no EXPRESS schema.
//
// # Scope: pure SPF, not the schema
//
// A STEP file is purely positional: a record #5=IFCWALL('guid',#6,'name',...)
// stores attributes by position, never by name. This package exposes exactly what
// is recoverable from the raw stream; naming and type-hierarchy features are a
// separate schema layer built on top of it.
//
//	In scope (pure SPF, no schema)          Out of scope (needs the EXPRESS schema)
//	----------------------------            ---------------------------------------
//	attribute by INDEX  (inst.Get(i))       attribute by NAME  (inst.GlobalId)
//	type keyword        (inst.Type/IsA)     is_a(supertype), subtype expansion
//	forward refs        (Value.Ref)         named inverse attrs (.IsDecomposedBy)
//	inverse graph       (File.Inverse)      derived-attribute formulas
//	traverse, by-id, by-type (exact)        by_guid, create_entity by name
//
// IsA and ByType are exact-type only. Inverse exposes the raw referrer graph;
// projecting it into named IFC inverse attributes is the schema layer's job.
//
// # Usage
//
//	f, err := step.ParseFile("model.ifc")
//	if err != nil { return err }
//	for _, wall := range f.ByType("IfcWall") {
//		if placement, ok := wall.Ref(5); ok {
//			_ = placement // #id resolved to *Instance
//		}
//	}
//	// who references this unit?
//	for _, referrer := range f.Inverse(f.ByType("IfcSiUnit")[0]) { _ = referrer }
//
// Parsing is eager and in-memory: the whole file is tokenized into instances in
// two passes (record load, then reference resolution + inverse indexing). Dangling
// references are non-fatal and surface via File.Warnings.
package step
