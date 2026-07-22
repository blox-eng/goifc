package step

import (
	"fmt"
	"io"
	"os"
	"strconv"
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
// entity graph. It runs two passes: pass 1 tokenizes the HEADER and every DATA
// record into instances (references captured but unresolved) and builds the id and
// type indexes; pass 2 resolves references to instance pointers and builds the
// inverse index. Dangling references are non-fatal warnings.
func ParseBytes(src []byte) (*File, error) {
	f := &File{
		byID:    make(map[uint32]*Instance),
		byType:  make(map[string][]*Instance),
		inverse: make(map[uint32][]InverseRef),
	}
	p := &parser{s: NewScanner(src), f: f, intern: make(map[string]string)}
	if err := p.parseDocument(); err != nil {
		// Errors propagate up without further scanning, so Pos() is at or just past
		// the offending token — a good-enough error location. Attach it once here.
		return nil, &ParseError{Offset: p.s.Pos(), Err: err}
	}
	resolveAndIndex(f)
	return f, nil
}

type parser struct {
	s      *Scanner
	f      *File
	intern map[string]string // type-keyword interning: one string per distinct type
}

// internType returns a shared, upper-cased copy of a type keyword so all instances
// of one type point at the same backing string.
func (p *parser) internType(raw []byte) string {
	up := strings.ToUpper(string(raw))
	if s, ok := p.intern[up]; ok {
		return s
	}
	p.intern[up] = up
	return up
}

// parseDocument walks ISO-10303-21 / HEADER / DATA / END sections.
func (p *parser) parseDocument() error {
	for {
		tok := p.s.Next()
		switch {
		case tok.Kind == TokEOF:
			return nil
		case tok.Kind == TokSemi:
			continue // stray section terminator (e.g. after ISO-10303-21, ENDSEC)
		case tok.Kind == TokKeyword:
			kw := strings.ToUpper(string(tok.Text))
			switch kw {
			case "ISO-10303-21", "ENDSEC", "HEADER":
				continue
			case "END-ISO-10303-21":
				return nil
			case "DATA":
				if err := p.parseData(); err != nil {
					return err
				}
			default:
				// A header record: KEYWORD(args); with no leading #id.
				if err := p.parseHeaderRecord(kw); err != nil {
					return err
				}
			}
		case tok.Kind == TokRef:
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
	args, err := parseArgs(p.s)
	if err != nil {
		return err
	}
	if semi := p.s.Next(); semi.Kind != TokSemi {
		return fmt.Errorf("step: expected ';' after header record %s, got %v", kw, semi.Kind)
	}
	switch kw {
	case "FILE_DESCRIPTION":
		if len(args) > 0 {
			p.f.Head.Description = flattenStrings(args[0])
		}
		if len(args) > 1 && args[1].Kind == KindString {
			p.f.Head.ImplementationLevel = args[1].Str
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
	return nil
}

// parseData consumes DATA-section instance records until ENDSEC.
func (p *parser) parseData() error {
	semi := p.s.Next()
	if semi.Kind != TokSemi {
		return fmt.Errorf("step: expected ';' after DATA, got %v", semi.Kind)
	}
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
	id, err := strconv.ParseUint(string(ref.Text), 10, 32)
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
	args, err := parseArgs(p.s)
	if err != nil {
		return fmt.Errorf("step: #%d %s: %w", id, typ, err)
	}
	if semi := p.s.Next(); semi.Kind != TokSemi {
		return fmt.Errorf("step: expected ';' after #%d %s, got %v", id, typ, semi.Kind)
	}
	p.register(&Instance{id: id, typ: typ, args: args, file: p.f}, nil)
	return nil
}

// finishComplexInstance completes "#id=(TYPEA(args)TYPEB(args)...);" — the opening
// '(' of the complex group has already been consumed. Part attribute lists are
// concatenated (a complex instance's attributes are the union of its parts) and the
// instance is indexed under every part type.
func (p *parser) finishComplexInstance(id uint32) error {
	var parts []string
	var allArgs []Value
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
		args, err := parseArgs(p.s)
		if err != nil {
			return fmt.Errorf("step: complex #%d %s: %w", id, kw, err)
		}
		parts = append(parts, kw)
		allArgs = append(allArgs, args...)
	}
	if len(parts) == 0 {
		return fmt.Errorf("step: empty complex instance #%d", id)
	}
	if semi := p.s.Next(); semi.Kind != TokSemi {
		return fmt.Errorf("step: expected ';' after complex #%d, got %v", id, semi.Kind)
	}
	p.register(&Instance{id: id, typ: parts[0], args: allArgs, file: p.f}, parts)
	return nil
}

// register adds inst to the id index, insertion order, and the type index under
// each of its types (parts is nil for a simple instance, or the full part list for
// a complex one; extra part types are recorded for IsA).
func (p *parser) register(inst *Instance, parts []string) {
	p.f.byID[inst.id] = inst
	p.f.order = append(p.f.order, inst.id)
	if len(parts) <= 1 {
		p.f.byType[inst.typ] = append(p.f.byType[inst.typ], inst)
		return
	}
	for _, pt := range parts {
		p.f.byType[pt] = append(p.f.byType[pt], inst)
	}
	if p.f.complexTypes == nil {
		p.f.complexTypes = make(map[uint32][]string)
	}
	p.f.complexTypes[inst.id] = parts
}

// headerTopLevelStrings maps a header record's top-level args to strings without
// recursing into sub-lists: a string arg becomes its value, any other kind (list,
// $, ...) becomes "". This preserves positional field indices.
func headerTopLevelStrings(args []Value) []string {
	out := make([]string, len(args))
	for i, v := range args {
		if v.Kind == KindString {
			out[i] = v.Str
		}
	}
	return out
}

// flattenStrings collects the string values of a value (a list, or a scalar
// string) into a flat slice, preserving order. Non-string members become "".
func flattenStrings(v Value) []string {
	switch v.Kind {
	case KindString:
		return []string{v.Str}
	case KindList:
		out := make([]string, 0, len(v.List))
		for _, c := range v.List {
			switch c.Kind {
			case KindString:
				out = append(out, c.Str)
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
