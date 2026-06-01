package stat

import (
	"fmt"
	"strconv"
)

// Parse turns a statistical-DSL string into a StatConfig. It is strict:
// any shape it doesn't recognise is an error (so a builder can surface a
// clean "couldn't load" flow rather than silently misreading). Parse is
// the inverse of Compile under the round-trip-identity contract.
func Parse(src string) (StatConfig, error) {
	toks, err := scanDSL(src)
	if err != nil {
		return StatConfig{}, err
	}
	p := &dslParser{toks: toks}
	cfg, err := p.object()
	if err != nil {
		return StatConfig{}, err
	}
	if p.peek().kind != tkEOF {
		return StatConfig{}, fmt.Errorf("stat dsl: unexpected trailing input near %q", p.peek().val)
	}
	if len(cfg.Measures) == 0 {
		return StatConfig{}, fmt.Errorf("stat dsl: at least one measure required")
	}
	return cfg, nil
}

// ── tokenizer ────────────────────────────────────────────────────────

type tokKind int

const (
	tkEOF tokKind = iota
	tkIdent
	tkString
	tkNumber
	tkLParen
	tkRParen
	tkLBrack
	tkRBrack
	tkComma
	tkBin // "@year" etc; val is the bin name without the @
)

type token struct {
	kind tokKind
	val  string
}

func isDigit(c byte) bool      { return c >= '0' && c <= '9' }
func isIdentStart(c byte) bool { return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }
func isIdentChar(c byte) bool  { return isIdentStart(c) || isDigit(c) }

func scanDSL(src string) ([]token, error) {
	var toks []token
	i, n := 0, len(src)
	for i < n {
		c := src[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '(':
			toks = append(toks, token{tkLParen, "("})
			i++
		case c == ')':
			toks = append(toks, token{tkRParen, ")"})
			i++
		case c == '[':
			toks = append(toks, token{tkLBrack, "["})
			i++
		case c == ']':
			toks = append(toks, token{tkRBrack, "]"})
			i++
		case c == ',':
			toks = append(toks, token{tkComma, ","})
			i++
		case c == '@':
			i++
			start := i
			for i < n && isIdentChar(src[i]) {
				i++
			}
			if start == i {
				return nil, fmt.Errorf("stat dsl: expected a bin name after '@'")
			}
			toks = append(toks, token{tkBin, src[start:i]})
		case c == '"':
			v, ni, err := scanString(src, i)
			if err != nil {
				return nil, err
			}
			toks = append(toks, token{tkString, v})
			i = ni
		case isDigit(c) || c == '.':
			start := i
			for i < n && (isDigit(src[i]) || src[i] == '.') {
				i++
			}
			toks = append(toks, token{tkNumber, src[start:i]})
		case isIdentStart(c):
			start := i
			for i < n && isIdentChar(src[i]) {
				i++
			}
			toks = append(toks, token{tkIdent, src[start:i]})
		default:
			return nil, fmt.Errorf("stat dsl: unexpected character %q", string(c))
		}
	}
	return append(toks, token{tkEOF, ""}), nil
}

// scanString reads a JSON/Go-quoted string starting at the opening quote
// (src[start] == '"') and returns the unquoted value plus the index after
// the closing quote.
func scanString(src string, start int) (string, int, error) {
	i, n := start+1, len(src)
	for i < n {
		if src[i] == '\\' {
			i += 2
			continue
		}
		if src[i] == '"' {
			v, err := strconv.Unquote(src[start : i+1])
			if err != nil {
				return "", 0, fmt.Errorf("stat dsl: bad string literal: %w", err)
			}
			return v, i + 1, nil
		}
		i++
	}
	return "", 0, fmt.Errorf("stat dsl: unterminated string literal")
}

// ── recursive-descent parser ─────────────────────────────────────────

type dslParser struct {
	toks []token
	pos  int
}

func (p *dslParser) peek() token { return p.toks[p.pos] }
func (p *dslParser) advance() token {
	t := p.toks[p.pos]
	if p.pos < len(p.toks)-1 {
		p.pos++
	}
	return t
}

func (p *dslParser) expect(k tokKind, what string) (token, error) {
	t := p.peek()
	if t.kind != k {
		return token{}, fmt.Errorf("stat dsl: expected %s, got %q", what, t.val)
	}
	return p.advance(), nil
}

func (p *dslParser) object() (StatConfig, error) {
	var cfg StatConfig
	m, err := p.measure()
	if err != nil {
		return cfg, err
	}
	cfg.Measures = append(cfg.Measures, m)
	for p.peek().kind == tkComma {
		p.advance()
		m, err := p.measure()
		if err != nil {
			return cfg, err
		}
		cfg.Measures = append(cfg.Measures, m)
	}
	if p.peek().kind == tkIdent && p.peek().val == "by" {
		p.advance()
		d, err := p.dimension()
		if err != nil {
			return cfg, err
		}
		cfg.Dimensions = append(cfg.Dimensions, d)
		for p.peek().kind == tkComma {
			p.advance()
			d, err := p.dimension()
			if err != nil {
				return cfg, err
			}
			cfg.Dimensions = append(cfg.Dimensions, d)
		}
	}
	if p.peek().kind == tkIdent && p.peek().val == "where" {
		p.advance()
		f, err := p.filter()
		if err != nil {
			return cfg, err
		}
		cfg.Filters = append(cfg.Filters, f)
		for p.peek().kind == tkIdent && p.peek().val == "and" {
			p.advance()
			f, err := p.filter()
			if err != nil {
				return cfg, err
			}
			cfg.Filters = append(cfg.Filters, f)
		}
	}
	// Trailing config clauses in any author order: pct at most once, scale any
	// number of times (factors multiply) but no duplicate name. Compile emits
	// scales (in order) before pct, so round-trip stays stable.
	seenPct := false
	seenScale := map[string]bool{}
	for p.peek().kind == tkIdent && (p.peek().val == "pct" || p.peek().val == "scale") {
		kw := p.peek().val
		p.advance()
		switch kw {
		case "pct":
			if seenPct {
				return cfg, fmt.Errorf("stat dsl: duplicate %q clause", kw)
			}
			seenPct = true
			bt := p.peek()
			if bt.kind != tkIdent {
				return cfg, fmt.Errorf("stat dsl: expected a percent base after 'pct', got %q", bt.val)
			}
			base := PercentBase(bt.val)
			if !validPercentBases[base] {
				return cfg, fmt.Errorf("stat dsl: invalid percent base %q (want distribution/forms/none)", bt.val)
			}
			p.advance()
			cfg.Percent = base
		case "scale":
			// A quoted name (object names may contain hyphens, not ident chars).
			nt, err := p.expect(tkString, "a quoted scaling object name")
			if err != nil {
				return cfg, err
			}
			if nt.val == "" {
				return cfg, fmt.Errorf("stat dsl: scale name must not be empty")
			}
			if seenScale[nt.val] {
				return cfg, fmt.Errorf("stat dsl: duplicate scale %q", nt.val)
			}
			seenScale[nt.val] = true
			cfg.Scales = append(cfg.Scales, nt.val)
		}
	}
	return cfg, nil
}

// filter parses `source op value`: an eq/ne op takes a quoted string;
// an lt/le/gt/ge op takes a number.
func (p *dslParser) filter() (Filter, error) {
	src, err := p.source()
	if err != nil {
		return Filter{}, err
	}
	ot := p.peek()
	if ot.kind != tkIdent {
		return Filter{}, fmt.Errorf("stat dsl: expected a filter operator (eq/ne/lt/le/gt/ge), got %q", ot.val)
	}
	op := FilterOp(ot.val)
	p.advance()
	switch {
	case equalityOps[op]:
		v, err := p.expect(tkString, "a quoted filter value")
		if err != nil {
			return Filter{}, err
		}
		return Filter{Source: src, Op: op, Value: v.val}, nil
	case comparisonOps[op]:
		nt, err := p.expect(tkNumber, "a numeric filter value")
		if err != nil {
			return Filter{}, err
		}
		if _, err := strconv.ParseFloat(nt.val, 64); err != nil {
			return Filter{}, fmt.Errorf("stat dsl: bad numeric value %q", nt.val)
		}
		return Filter{Source: src, Op: op, Value: nt.val}, nil
	default:
		return Filter{}, fmt.Errorf("stat dsl: unknown filter operator %q", op)
	}
}

func (p *dslParser) measure() (Measure, error) {
	t := p.peek()
	if t.kind != tkIdent {
		return Measure{}, fmt.Errorf("stat dsl: expected a measure, got %q", t.val)
	}
	op := MeasureOp(t.val)
	p.advance()
	if _, err := p.expect(tkLParen, "'('"); err != nil {
		return Measure{}, err
	}
	switch {
	case op == OpCount || op == OpRecords:
		if _, err := p.expect(tkRParen, "')'"); err != nil {
			return Measure{}, err
		}
		return Measure{Op: op}, nil
	case op == OpPercentile:
		src, err := p.fieldSource(string(op))
		if err != nil {
			return Measure{}, err
		}
		if _, err := p.expect(tkComma, "',' before percentile value"); err != nil {
			return Measure{}, err
		}
		nt, err := p.expect(tkNumber, "a percentile number")
		if err != nil {
			return Measure{}, err
		}
		f, err := strconv.ParseFloat(nt.val, 64)
		if err != nil {
			return Measure{}, fmt.Errorf("stat dsl: bad percentile number %q", nt.val)
		}
		if _, err := p.expect(tkRParen, "')'"); err != nil {
			return Measure{}, err
		}
		return Measure{Op: OpPercentile, Source: &src, Arg: &f}, nil
	case reduceOps[op]:
		src, err := p.fieldSource(string(op))
		if err != nil {
			return Measure{}, err
		}
		if _, err := p.expect(tkRParen, "')'"); err != nil {
			return Measure{}, err
		}
		return Measure{Op: op, Source: &src}, nil
	default:
		return Measure{}, fmt.Errorf("stat dsl: unknown measure %q", op)
	}
}

// fieldSource parses a source and requires it to be a field (reduce and
// percentile measures don't accept a facet source).
func (p *dslParser) fieldSource(op string) (SourceRef, error) {
	src, err := p.source()
	if err != nil {
		return SourceRef{}, err
	}
	if src.Kind != SourceField {
		return SourceRef{}, fmt.Errorf("stat dsl: %s source must be a field", op)
	}
	return src, nil
}

func (p *dslParser) source() (SourceRef, error) {
	t := p.peek()
	if t.kind != tkIdent {
		return SourceRef{}, fmt.Errorf("stat dsl: expected F[...] or Facet[...], got %q", t.val)
	}
	switch t.val {
	case "F":
		p.advance()
		key, err := p.bracketString()
		if err != nil {
			return SourceRef{}, err
		}
		ref := SourceRef{Kind: SourceField, Key: key}
		if p.peek().kind == tkLBrack {
			col, err := p.bracketString()
			if err != nil {
				return SourceRef{}, err
			}
			ref.Column = col
		}
		return ref, nil
	case "Facet":
		p.advance()
		key, err := p.bracketString()
		if err != nil {
			return SourceRef{}, err
		}
		return SourceRef{Kind: SourceFacet, Key: key}, nil
	default:
		return SourceRef{}, fmt.Errorf("stat dsl: expected F[...] or Facet[...], got %q", t.val)
	}
}

// bracketString consumes a `[ "string" ]` group and returns the string.
func (p *dslParser) bracketString() (string, error) {
	if _, err := p.expect(tkLBrack, "'['"); err != nil {
		return "", err
	}
	s, err := p.expect(tkString, "a quoted key")
	if err != nil {
		return "", err
	}
	if _, err := p.expect(tkRBrack, "']'"); err != nil {
		return "", err
	}
	return s.val, nil
}

func (p *dslParser) dimension() (Dimension, error) {
	src, err := p.source()
	if err != nil {
		return Dimension{}, err
	}
	d := Dimension{Source: src}
	if p.peek().kind == tkBin {
		b := Bin(p.peek().val)
		if !validBins[b] {
			return Dimension{}, fmt.Errorf("stat dsl: invalid bin %q (want year/month/day)", p.peek().val)
		}
		p.advance()
		d.Bin = b
	}
	if p.peek().kind == tkIdent && p.peek().val == "top" {
		p.advance()
		nt, err := p.expect(tkNumber, "a top-N count")
		if err != nil {
			return Dimension{}, err
		}
		n, err := strconv.Atoi(nt.val)
		if err != nil {
			return Dimension{}, fmt.Errorf("stat dsl: bad top count %q", nt.val)
		}
		d.Top = n
	}
	return d, nil
}
