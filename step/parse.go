package step

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// ParseFile reads and parses a STEP/SPF file from path.
func ParseFile(path string) (*File, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseBytes(src)
}

// Parse reads all of r and parses it as a STEP/SPF stream.
func Parse(r io.Reader) (*File, error) {
	src, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return ParseBytes(src)
}

// ParseBytes parses an in-memory STEP/SPF (ISO 10303-21) document into a navigable
// entity graph. It runs two passes: pass 1 reads the HEADER and every DATA record
// into instances, with references kept as ids, across all cores for a large
// file; pass 2 builds the id, type and inverse indexes. A reference is looked up
// by id when it is read. Dangling references are non-fatal warnings.
func ParseBytes(src []byte) (*File, error) {
	if f, ok := parseParallel(src); ok {
		return f, nil
	}
	return parseSerial(src)
}

// parseSerial parses src with one parser: the reference parseParallel must
// reproduce, and the path that reports every parse error.
func parseSerial(src []byte) (*File, error) {
	f := &File{}
	p := newParser(src, f, 0, len(src))
	if err := p.parseDocument(); err != nil {
		// Errors propagate up without further scanning, so Pos() is at or just past
		// the offending token — a good-enough error location. Attach it once here.
		return nil, &ParseError{Offset: p.s.Pos(), Err: err}
	}
	finish(f, []*parser{p})
	return f, nil
}

// parser reads one stretch of a file into its own slab: the whole file, or one
// chunk of the DATA section when parsing in parallel.
type parser struct {
	s       *Scanner
	f       *File
	slab    uint16
	intern  map[string]string // type-keyword interning: one string per distinct type
	stack   []Value           // values of the lists being parsed, innermost last
	vals    []Value
	strs    []byte // frozen into the slab by finish
	insts   []Instance
	complex map[uint32][]string
	header  []headerRecord // read before the arena is frozen, decoded after

	// stopAtData makes parseDocument return right after "DATA;", with atData
	// set, so the records that follow can be split across parsers.
	stopAtData bool
	atData     bool
}

// newParser returns a parser for src with slabs sized for n bytes of it. An IFC
// instance averages 55-70 bytes of text and a value 13-14, so the slabs rarely
// grow; over-sizing costs address space, not resident memory.
func newParser(src []byte, f *File, slab uint16, n int) *parser {
	return &parser{
		s:      NewScanner(src),
		f:      f,
		slab:   slab,
		intern: make(map[string]string),
		vals:   make([]Value, 0, n/12+64),
		strs:   make([]byte, 0, n/4+64),
		insts:  make([]Instance, 0, n/48+16),
	}
}

// headerRecord is a HEADER entry kept until finish, when its strings exist.
type headerRecord struct {
	kw   string
	args Value
}

// internType returns a shared, upper-cased copy of a type keyword so all instances
// of one type point at the same backing string.
// The map is keyed by the keyword as written as well as upper-cased, and looked
// up with string(raw), which Go does without allocating: one allocation per
// distinct spelling instead of one per instance.
func (p *parser) internType(raw []byte) string {
	if s, ok := p.intern[string(raw)]; ok {
		return s
	}
	up := strings.ToUpper(string(raw))
	if s, ok := p.intern[up]; ok {
		up = s
	}
	p.intern[up] = up
	p.intern[string(raw)] = up
	return up
}

// parseDocument walks ISO-10303-21 / HEADER / DATA / END sections.
func (p *parser) parseDocument() error {
	for {
		tok := p.s.Next()
		switch tok.Kind {
		case TokEOF:
			return nil
		case TokSemi:
			continue // stray section terminator (e.g. after ISO-10303-21, ENDSEC)
		case TokKeyword:
			kw := strings.ToUpper(string(tok.Text))
			switch kw {
			case "ISO-10303-21", "ENDSEC", "HEADER":
				continue
			case "END-ISO-10303-21":
				return nil
			case "DATA":
				if p.stopAtData {
					if semi := p.s.Next(); semi.Kind != TokSemi {
						return fmt.Errorf("step: expected ';' after DATA, got %v", semi.Kind)
					}
					p.atData = true
					return nil
				}
				if err := p.parseData(); err != nil {
					return err
				}
			default:
				// A header record: KEYWORD(args); with no leading #id.
				if err := p.parseHeaderRecord(kw); err != nil {
					return err
				}
			}
		case TokRef:
			if err := p.parseInstance(tok); err != nil {
				return err
			}
		default:
			return fmt.Errorf("step: unexpected token %v at top level", tok.Kind)
		}
	}
}

// parseHeaderRecord reads a HEADER entry (FILE_DESCRIPTION/FILE_NAME/FILE_SCHEMA)
// whose keyword has already been consumed. The '(' follows.
func (p *parser) parseHeaderRecord(kw string) error {
	open := p.s.Next()
	if open.Kind != TokLParen {
		return fmt.Errorf("step: expected '(' after header keyword %s, got %v", kw, open.Kind)
	}
	mark := len(p.stack)
	if err := p.parseArgs(); err != nil {
		return err
	}
	if semi := p.s.Next(); semi.Kind != TokSemi {
		return fmt.Errorf("step: expected ';' after header record %s, got %v", kw, semi.Kind)
	}
	p.header = append(p.header, headerRecord{kw, p.closeList(mark, KindList)})
	return nil
}

// setHeader decodes one HEADER record into f.Head.
func (p *parser) setHeader(kw string, args []Value) {
	switch kw {
	case "FILE_DESCRIPTION":
		if len(args) > 0 {
			p.f.Head.Description = flattenStrings(args[0])
		}
		if len(args) > 1 && args[1].kind == KindString {
			p.f.Head.ImplementationLevel = args[1].Str()
		}
	case "FILE_NAME":
		// FILE_NAME has 7 positional fields, two of which (author, organization)
		// are sub-lists. Keep top-level positions stable — do NOT inline the
		// sub-lists (a multi-author file would otherwise shift every later field).
		p.f.Head.Name = headerTopLevelStrings(args)
	case "FILE_SCHEMA":
		if len(args) > 0 {
			p.f.Head.Schema = flattenStrings(args[0])
		}
	}
}

// parseData consumes DATA-section instance records until ENDSEC.
func (p *parser) parseData() error {
	semi := p.s.Next()
	if semi.Kind != TokSemi {
		return fmt.Errorf("step: expected ';' after DATA, got %v", semi.Kind)
	}
	return p.parseRecords()
}

// parseRecords consumes DATA-section instance records until ENDSEC.
func (p *parser) parseRecords() error {
	for {
		tok := p.s.Next()
		switch tok.Kind {
		case TokRef:
			if err := p.parseInstance(tok); err != nil {
				return err
			}
		case TokKeyword:
			if strings.EqualFold(string(tok.Text), "ENDSEC") {
				return nil
			}
			return fmt.Errorf("step: unexpected keyword %q in DATA section", tok.Text)
		case TokSemi:
			continue
		case TokEOF:
			return fmt.Errorf("step: unexpected EOF in DATA section")
		default:
			return fmt.Errorf("step: unexpected token %v in DATA section", tok.Kind)
		}
	}
}

// parseInstance reads a DATA record given the leading #id token. It dispatches on
// the token after '=': a keyword begins a simple instance "#id=KEYWORD(args);"; a
// '(' begins an ISO-10303-21 complex instance "#id=(TYPEA(args)TYPEB(args)...);".
func (p *parser) parseInstance(ref Token) error {
	id, err := parseUint32(ref.Text)
	if err != nil {
		return fmt.Errorf("step: bad instance id #%s: %w", ref.Text, err)
	}
	if eq := p.s.Next(); eq.Kind != TokEquals {
		return fmt.Errorf("step: expected '=' after #%d, got %v", id, eq.Kind)
	}
	next := p.s.Next()
	switch next.Kind {
	case TokKeyword:
		return p.finishSimpleInstance(uint32(id), next)
	case TokLParen:
		return p.finishComplexInstance(uint32(id))
	default:
		return fmt.Errorf("step: expected entity keyword or '(' for #%d, got %v", id, next.Kind)
	}
}

// finishSimpleInstance completes "#id=KEYWORD(args);" given the keyword token.
func (p *parser) finishSimpleInstance(id uint32, kwTok Token) error {
	typ := p.internType(kwTok.Text)
	if open := p.s.Next(); open.Kind != TokLParen {
		return fmt.Errorf("step: expected '(' for #%d %s, got %v", id, typ, open.Kind)
	}
	mark := len(p.stack)
	if err := p.parseArgs(); err != nil {
		return fmt.Errorf("step: #%d %s: %w", id, typ, err)
	}
	if semi := p.s.Next(); semi.Kind != TokSemi {
		return fmt.Errorf("step: expected ';' after #%d %s, got %v", id, typ, semi.Kind)
	}
	p.register(id, typ, mark, nil)
	return nil
}

// finishComplexInstance completes "#id=(TYPEA(args)TYPEB(args)...);" — the opening
// '(' of the complex group has already been consumed. Part attribute lists are
// concatenated (a complex instance's attributes are the union of its parts) and the
// instance is indexed under every part type.
func (p *parser) finishComplexInstance(id uint32) error {
	var parts []string
	mark := len(p.stack)
	for {
		tok := p.s.Next()
		if tok.Kind == TokRParen {
			break
		}
		if tok.Kind != TokKeyword {
			return fmt.Errorf("step: expected part type in complex #%d, got %v", id, tok.Kind)
		}
		kw := p.internType(tok.Text)
		if open := p.s.Next(); open.Kind != TokLParen {
			return fmt.Errorf("step: expected '(' after %s in complex #%d, got %v", kw, id, open.Kind)
		}
		// Part attribute lists land on the stack back to back, which is the
		// concatenation a complex instance's attributes are.
		if err := p.parseArgs(); err != nil {
			return fmt.Errorf("step: complex #%d %s: %w", id, kw, err)
		}
		parts = append(parts, kw)
	}
	if len(parts) == 0 {
		return fmt.Errorf("step: empty complex instance #%d", id)
	}
	if semi := p.s.Next(); semi.Kind != TokSemi {
		return fmt.Errorf("step: expected ';' after complex #%d, got %v", id, semi.Kind)
	}
	p.register(id, parts[0], mark, parts)
	return nil
}

// register moves the instance's arguments, p.stack[mark:], into the value slab
// and appends the instance. parts is nil for a simple instance, or the full part
// list for a complex one; extra part types are recorded for IsA. Indexing by id
// and type waits for finish, when the instance slab stops moving.
func (p *parser) register(id uint32, typ string, mark int, parts []string) {
	args := p.closeList(mark, KindList)
	p.insts = append(p.insts, Instance{typ: typ, file: p.f, id: id, start: uint32(args.x & offMask), n: args.n, slab: p.slab})
	if len(parts) > 1 {
		if p.complex == nil {
			p.complex = make(map[uint32][]string)
		}
		p.complex[id] = parts
	}
}

// headerTopLevelStrings maps a header record's top-level args to strings without
// recursing into sub-lists: a string arg becomes its value, any other kind (list,
// $, ...) becomes "". This preserves positional field indices.
func headerTopLevelStrings(args []Value) []string {
	out := make([]string, len(args))
	for i, v := range args {
		if v.kind == KindString {
			out[i] = v.Str()
		}
	}
	return out
}

// flattenStrings collects the string values of a value (a list, or a scalar
// string) into a flat slice, preserving order. Non-string members become "".
func flattenStrings(v Value) []string {
	switch v.kind {
	case KindString:
		return []string{v.Str()}
	case KindList:
		out := make([]string, 0, len(v.List()))
		for _, c := range v.List() {
			switch c.kind {
			case KindString:
				out = append(out, c.Str())
			case KindList:
				out = append(out, flattenStrings(c)...)
			default:
				out = append(out, "")
			}
		}
		return out
	default:
		return nil
	}
}
