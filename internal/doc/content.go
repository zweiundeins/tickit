package doc

import (
	"slices"
	"strconv"
	"strings"
)

// ContentMatch is a state of a node type's content expression, compiled to
// a deterministic automaton as prosemirror-model's ContentMatch is. The
// order of the edges matters: it follows the expression and schema order.
type ContentMatch struct {
	validEnd bool
	next     []matchEdge
}

type matchEdge struct {
	typ  *NodeType
	next *ContentMatch
}

var emptyMatch = &ContentMatch{validEnd: true}

// ValidEnd reports whether the content may end in this state.
func (m *ContentMatch) ValidEnd() bool { return m.validEnd }

// MatchType returns the state after a node of type t, or nil.
func (m *ContentMatch) MatchType(t *NodeType) *ContentMatch {
	for i := range m.next {
		if m.next[i].typ == t {
			return m.next[i].next
		}
	}
	return nil
}

func (m *ContentMatch) matchFragment(f Fragment, start, end int) *ContentMatch {
	cur := m
	for i := start; cur != nil && i < end; i++ {
		cur = cur.MatchType(f.nodes[i].typ)
	}
	return cur
}

func (m *ContentMatch) inlineContent() bool {
	return len(m.next) != 0 && !m.next[0].typ.isBlock
}

func (m *ContentMatch) compatible(other *ContentMatch) bool {
	for i := range m.next {
		for j := range other.next {
			if m.next[i].typ == other.next[j].typ {
				return true
			}
		}
	}
	return false
}

// DefaultType returns the first type at this state that can be made
// without attributes, or nil.
func (m *ContentMatch) DefaultType() *NodeType {
	for _, e := range m.next {
		if !(e.typ.isText || e.typ.hasRequiredAttrs()) {
			return e.typ
		}
	}
	return nil
}

// String describes the automaton as prosemirror-model's toString does.
func (m *ContentMatch) String() string {
	var seen []*ContentMatch
	var scan func(*ContentMatch)
	scan = func(c *ContentMatch) {
		seen = append(seen, c)
		for _, e := range c.next {
			if !slices.Contains(seen, e.next) {
				scan(e.next)
			}
		}
	}
	scan(m)
	var b strings.Builder
	for i, c := range seen {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(strconv.Itoa(i))
		if c.validEnd {
			b.WriteString("* ")
		} else {
			b.WriteString("  ")
		}
		for j, e := range c.next {
			if j > 0 {
				b.WriteString(", ")
			}
			b.WriteString(e.typ.name + "->" + strconv.Itoa(slices.Index(seen, e.next)))
		}
	}
	return b.String()
}

type tokenStream struct {
	str    string
	tokens []string
	pos    int
	schema *Schema
	inline int // -1 unknown, 0 block, 1 inline
}

// tokenize splits a content expression as V8 runs
// string.split(/\s*(?=\b|\W|$)/): words, and every other character but
// white space on its own.
func tokenize(s string) []string {
	var out []string
	isWord := func(c byte) bool {
		return c == '_' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
	}
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v':
			i++
		case isWord(c):
			j := i
			for j < len(s) && isWord(s[j]) {
				j++
			}
			out = append(out, s[i:j])
			i = j
		default:
			out = append(out, s[i:i+1])
			i++
		}
	}
	return out
}

func (ts *tokenStream) next() string {
	if ts.pos < len(ts.tokens) {
		return ts.tokens[ts.pos]
	}
	return ""
}

func (ts *tokenStream) eat(tok string) bool {
	if ts.next() == tok {
		ts.pos++
		return true
	}
	return false
}

func (ts *tokenStream) err(msg string) error {
	return &ModelError{Msg: msg + " (in content expression '" + ts.str + "')"}
}

type exprKind int

const (
	exprChoice exprKind = iota
	exprSeq
	exprPlus
	exprStar
	exprOpt
	exprRange
	exprName
)

type expr struct {
	kind     exprKind
	exprs    []*expr
	expr     *expr
	min, max int
	typ      *NodeType
}

func parseContentMatch(s string, schema *Schema) (*ContentMatch, error) {
	ts := &tokenStream{str: s, tokens: tokenize(s), schema: schema, inline: -1}
	if len(ts.tokens) == 0 {
		return emptyMatch, nil
	}
	e, err := parseExpr(ts)
	if err != nil {
		return nil, err
	}
	if ts.next() != "" {
		return nil, ts.err("Unexpected trailing text")
	}
	m := dfa(nfa(e))
	if err := checkForDeadEnds(m, ts); err != nil {
		return nil, err
	}
	return m, nil
}

func parseExpr(ts *tokenStream) (*expr, error) {
	var exprs []*expr
	for {
		e, err := parseExprSeq(ts)
		if err != nil {
			return nil, err
		}
		exprs = append(exprs, e)
		if !ts.eat("|") {
			break
		}
	}
	if len(exprs) == 1 {
		return exprs[0], nil
	}
	return &expr{kind: exprChoice, exprs: exprs}, nil
}

func parseExprSeq(ts *tokenStream) (*expr, error) {
	var exprs []*expr
	for {
		e, err := parseExprSubscript(ts)
		if err != nil {
			return nil, err
		}
		exprs = append(exprs, e)
		if n := ts.next(); n == "" || n == ")" || n == "|" {
			break
		}
	}
	if len(exprs) == 1 {
		return exprs[0], nil
	}
	return &expr{kind: exprSeq, exprs: exprs}, nil
}

func parseExprSubscript(ts *tokenStream) (*expr, error) {
	e, err := parseExprAtom(ts)
	if err != nil {
		return nil, err
	}
	for {
		switch {
		case ts.eat("+"):
			e = &expr{kind: exprPlus, expr: e}
		case ts.eat("*"):
			e = &expr{kind: exprStar, expr: e}
		case ts.eat("?"):
			e = &expr{kind: exprOpt, expr: e}
		case ts.eat("{"):
			if e, err = parseExprRange(ts, e); err != nil {
				return nil, err
			}
		default:
			return e, nil
		}
	}
}

func parseNum(ts *tokenStream) (int, error) {
	n := ts.next()
	for i := 0; i < len(n); i++ {
		if n[i] < '0' || n[i] > '9' {
			return 0, ts.err("Expected number, got '" + n + "'")
		}
	}
	v, _ := strconv.Atoi(n)
	ts.pos++
	return v, nil
}

func parseExprRange(ts *tokenStream, e *expr) (*expr, error) {
	min, err := parseNum(ts)
	if err != nil {
		return nil, err
	}
	max := min
	if ts.eat(",") {
		if ts.next() != "}" {
			if max, err = parseNum(ts); err != nil {
				return nil, err
			}
		} else {
			max = -1
		}
	}
	if !ts.eat("}") {
		return nil, ts.err("Unclosed braced range")
	}
	return &expr{kind: exprRange, min: min, max: max, expr: e}, nil
}

func resolveName(ts *tokenStream, name string) ([]*NodeType, error) {
	if t := ts.schema.nodeByID[name]; t != nil {
		return []*NodeType{t}, nil
	}
	var out []*NodeType
	for _, t := range ts.schema.nodes {
		if t.isInGroup(name) {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return nil, ts.err("No node type or group '" + name + "' found")
	}
	return out, nil
}

func parseExprAtom(ts *tokenStream) (*expr, error) {
	if ts.eat("(") {
		e, err := parseExpr(ts)
		if err != nil {
			return nil, err
		}
		if !ts.eat(")") {
			return nil, ts.err("Missing closing paren")
		}
		return e, nil
	}
	n := ts.next()
	if n != "" && isWordToken(n) {
		types, err := resolveName(ts, n)
		if err != nil {
			return nil, err
		}
		var exprs []*expr
		for _, t := range types {
			in := 0
			if !t.isBlock {
				in = 1
			}
			if ts.inline == -1 {
				ts.inline = in
			} else if ts.inline != in {
				return nil, ts.err("Mixing inline and block content")
			}
			exprs = append(exprs, &expr{kind: exprName, typ: t})
		}
		ts.pos++
		if len(exprs) == 1 {
			return exprs[0], nil
		}
		return &expr{kind: exprChoice, exprs: exprs}, nil
	}
	return nil, ts.err("Unexpected token '" + n + "'")
}

func isWordToken(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c == '_' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
			return false
		}
	}
	return true
}

type nfaEdge struct {
	term *NodeType
	to   int // -1 until connected
}

// nfa builds prosemirror-model's NFA: state 0 is the entry, the last state
// the success state.
func nfa(e *expr) [][]*nfaEdge {
	states := [][]*nfaEdge{{}}
	node := func() int {
		states = append(states, nil)
		return len(states) - 1
	}
	edge := func(from, to int, term *NodeType) *nfaEdge {
		ed := &nfaEdge{term: term, to: to}
		states[from] = append(states[from], ed)
		return ed
	}
	connect := func(edges []*nfaEdge, to int) {
		for _, ed := range edges {
			ed.to = to
		}
	}
	var compile func(e *expr, from int) []*nfaEdge
	compile = func(e *expr, from int) []*nfaEdge {
		switch e.kind {
		case exprChoice:
			var out []*nfaEdge
			for _, x := range e.exprs {
				out = append(out, compile(x, from)...)
			}
			return out
		case exprSeq:
			for i := 0; ; i++ {
				next := compile(e.exprs[i], from)
				if i == len(e.exprs)-1 {
					return next
				}
				from = node()
				connect(next, from)
			}
		case exprStar:
			loop := node()
			edge(from, loop, nil)
			connect(compile(e.expr, loop), loop)
			return []*nfaEdge{edge(loop, -1, nil)}
		case exprPlus:
			loop := node()
			connect(compile(e.expr, from), loop)
			connect(compile(e.expr, loop), loop)
			return []*nfaEdge{edge(loop, -1, nil)}
		case exprOpt:
			return append([]*nfaEdge{edge(from, -1, nil)}, compile(e.expr, from)...)
		case exprRange:
			cur := from
			for i := 0; i < e.min; i++ {
				next := node()
				connect(compile(e.expr, cur), next)
				cur = next
			}
			if e.max == -1 {
				connect(compile(e.expr, cur), cur)
			} else {
				for i := e.min; i < e.max; i++ {
					next := node()
					edge(cur, next, nil)
					connect(compile(e.expr, cur), next)
					cur = next
				}
			}
			return []*nfaEdge{edge(cur, -1, nil)}
		default:
			return []*nfaEdge{edge(from, -1, e.typ)}
		}
	}
	connect(compile(e, 0), node())
	return states
}

// nullFrom returns the states reachable by null edges from node, leaving
// out states with a single null edge, in descending order.
func nullFrom(states [][]*nfaEdge, node int) []int {
	var result []int
	var scan func(int)
	scan = func(n int) {
		edges := states[n]
		if len(edges) == 1 && edges[0].term == nil {
			scan(edges[0].to)
			return
		}
		result = append(result, n)
		for _, ed := range edges {
			if ed.term == nil && !slices.Contains(result, ed.to) {
				scan(ed.to)
			}
		}
	}
	scan(node)
	slices.SortFunc(result, func(a, b int) int { return b - a })
	return result
}

func joinStates(states []int) string {
	var b strings.Builder
	for i, s := range states {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.Itoa(s))
	}
	return b.String()
}

func dfa(states [][]*nfaEdge) *ContentMatch {
	labeled := map[string]*ContentMatch{}
	var explore func(set []int) *ContentMatch
	explore = func(set []int) *ContentMatch {
		type out struct {
			term *NodeType
			set  []int
		}
		var outs []*out
		for _, n := range set {
			for _, ed := range states[n] {
				if ed.term == nil {
					continue
				}
				var o *out
				for _, x := range outs {
					if x.term == ed.term {
						o = x
					}
				}
				for _, m := range nullFrom(states, ed.to) {
					if o == nil {
						o = &out{term: ed.term}
						outs = append(outs, o)
					}
					if !slices.Contains(o.set, m) {
						o.set = append(o.set, m)
					}
				}
			}
		}
		state := &ContentMatch{validEnd: slices.Contains(set, len(states)-1)}
		labeled[joinStates(set)] = state
		for _, o := range outs {
			slices.SortFunc(o.set, func(a, b int) int { return b - a })
			next := labeled[joinStates(o.set)]
			if next == nil {
				next = explore(o.set)
			}
			state.next = append(state.next, matchEdge{typ: o.term, next: next})
		}
		return state
	}
	return explore(nullFrom(states, 0))
}

func checkForDeadEnds(m *ContentMatch, ts *tokenStream) error {
	work := []*ContentMatch{m}
	for i := 0; i < len(work); i++ {
		state := work[i]
		dead := !state.validEnd
		var nodes []string
		for _, e := range state.next {
			nodes = append(nodes, e.typ.name)
			if dead && !(e.typ.isText || e.typ.hasRequiredAttrs()) {
				dead = false
			}
			if !slices.Contains(work, e.next) {
				work = append(work, e.next)
			}
		}
		if dead {
			return ts.err("Only non-generatable nodes (" + strings.Join(nodes, ", ") + ") in a required position (see https://prosemirror.net/docs/guide/#generatable)")
		}
	}
	return nil
}
