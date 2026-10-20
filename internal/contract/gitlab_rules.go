package contract

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Which GitLab jobs run for a pipeline kind (spec 0055, "Pipeline kinds
// and rules (Q4)"). Each kind is read as one or more evaluation contexts:
// the values the variables the audit reads have in that pipeline. A merge
// request is its merge request pipeline or a branch pipeline of its
// source branch (GitLab shows that on the merge request); a push to the
// default branch is one branch pipeline. workflow:rules decide whether
// the pipeline exists, then a job's rules: (first match wins, when: never
// excludes) or its only/except.
//
// The conditions read form a closed list: $CI_PIPELINE_SOURCE == or != a
// quoted literal; $CI_MERGE_REQUEST_IID, $CI_COMMIT_BRANCH and
// $CI_COMMIT_TAG as presence tests; $CI_COMMIT_BRANCH or
// $CI_COMMIT_REF_NAME == or != $CI_DEFAULT_BRANCH or a quoted branch
// name; &&, || and parentheses over these; and only/except with
// merge_requests, branches, pushes, tags, schedules and branch names.
// Conditions are three-valued, so a condition the audit does not read
// decides nothing when the rest decides alone. A rule left undecided, or
// one with changes: or exists:, does not admit the job: with when: never
// it leaves the job out, otherwise a later rule may still admit it
// (evalRules). A job left out that way is reported with the rule quoted
// with its line.

// glRef is what a ref variable holds in a context.
type glRef int

const (
	// refNone: the variable is not set.
	refNone glRef = iota
	// refDefault: the default branch.
	refDefault
	// refOther: a branch that is neither the default branch nor any
	// branch a rule names.
	refOther
)

// glContext is the values of the variables the audit reads in one
// pipeline.
type glContext struct {
	// all is the every-job context: every include is followed and no rule
	// evaluated.
	all    bool
	source string
	// mr is whether $CI_MERGE_REQUEST_IID is set.
	mr bool
	// branch and ref are $CI_COMMIT_BRANCH and $CI_COMMIT_REF_NAME.
	branch, ref   glRef
	defaultBranch string
}

func (c *glContext) key() string {
	if c.all {
		return "all"
	}
	return fmt.Sprintf("%s/%t/%d/%d/%s", c.source, c.mr, c.branch, c.ref, c.defaultBranch)
}

// child is the context of a child pipeline: the parent's, triggered by it.
func (c *glContext) child() *glContext {
	if c.all {
		return c
	}
	out := *c
	out.source = "parent_pipeline"
	return &out
}

// contextsFor returns the contexts a pipeline kind is read in.
func contextsFor(kind PipelineKind, branch string) []*glContext {
	if kind == KindDefaultPush {
		return []*glContext{{source: "push", branch: refDefault, ref: refDefault, defaultBranch: branch}}
	}
	return []*glContext{
		{source: "merge_request_event", mr: true, branch: refNone, ref: refOther, defaultBranch: branch},
		{source: "push", branch: refOther, ref: refOther, defaultBranch: branch},
	}
}

// glWhy is why a job does not run in a context.
type glWhy struct {
	file string
	line int
	text string
	// unread is true when a condition the audit does not read (or
	// changes:, exists:) decided it, rather than conditions it read.
	unread bool
}

func (r *glReader) why(n *yaml.Node, unread bool, format string, args ...any) *glWhy {
	return &glWhy{file: r.origin[n], line: n.Line, text: fmt.Sprintf(format, args...), unread: unread}
}

// admitted decides whether a job runs in ctx: the pipeline's
// workflow:rules, then the job's rules: or only/except.
func (r *glReader) admitted(p *glPipeline, j *glJob, ctx *glContext) (bool, *glWhy) {
	if wr := mapValue(mapValue(p.root, "workflow"), "rules"); wr != nil {
		if ok, why := r.evalRules(wr, ctx, "workflow:rules"); !ok {
			return false, why
		}
	}
	if rules := mapValue(j.node, "rules"); rules != nil {
		return r.evalRules(rules, ctx, "rules")
	}
	if w := mapValue(j.node, "when"); w != nil && w.Value == "never" {
		return false, r.why(w, false, "when: never")
	}
	return r.onlyExcept(j.node, ctx)
}

// evalRules applies a rules: list: the first rule that matches decides,
// when: never excludes, and no match leaves the job (or pipeline) out.
//
// A rule the audit cannot decide (an if: it does not read, changes: or
// exists:) is unread. An unread rule with when: never might exclude the
// job, so the job is not admitted. Any other unread rule is passed over:
// had it matched, the job would run anyway, so a later rule that admits
// it decides. When no later rule admits it, the reason quotes the first
// unread rule.
func (r *glReader) evalRules(rules *yaml.Node, ctx *glContext, label string) (bool, *glWhy) {
	if rules.Kind != yaml.SequenceNode {
		return false, r.why(rules, true, "%s: rules that are not a list", label)
	}
	var unread *glWhy
	for _, rule := range flattenRules(rules, 0) {
		if rule.Kind != yaml.MappingNode {
			return false, r.why(rule, true, "%s: a rule that is not a map", label)
		}
		never := false
		if w := mapValue(rule, "when"); w != nil && w.Value == "never" {
			never = true
		}
		var why *glWhy
		if cond := mapValue(rule, "if"); cond != nil {
			switch evalExpression(scalarOr(cond, ""), ctx) {
			case triFalse:
				continue
			case triUnknown:
				why = r.why(cond, true, "%s: if: `%s` is a condition the audit does not read", label, scalarOr(cond, "(not a string)"))
			}
		}
		for _, k := range []string{"changes", "exists"} {
			if key := mapKey(rule, k); key != nil && why == nil {
				why = r.why(key, true, "%s: a rule with %s: does not count (the audit does not read which files change or exist)", label, k)
			}
		}
		switch {
		case why != nil && never:
			return false, why
		case why != nil:
			if unread == nil {
				unread = why
			}
			continue
		case never && unread != nil:
			return false, unread
		case never:
			return false, r.why(mapValue(rule, "when"), false, "%s: when: never excludes it", label)
		}
		return true, nil
	}
	if unread != nil {
		return false, unread
	}
	return false, r.why(rules, false, "%s: no rule admits it", label)
}

// flattenRules flattens a rules: list with nested lists (from !reference).
func flattenRules(n *yaml.Node, depth int) []*yaml.Node {
	var out []*yaml.Node
	for _, c := range n.Content {
		if c.Kind == yaml.SequenceNode && depth < 10 {
			out = append(out, flattenRules(c, depth+1)...)
			continue
		}
		out = append(out, c)
	}
	return out
}

// onlyExcept applies only: and except:. Without either, a job runs in
// branch and tag pipelines (GitLab's only: [branches, tags]).
func (r *glReader) onlyExcept(job *yaml.Node, ctx *glContext) (bool, *glWhy) {
	only, except := mapValue(job, "only"), mapValue(job, "except")
	if only == nil {
		if ctx.branch == refNone {
			return false, r.why(job, false, "with no rules and no only:, it runs in branch and tag pipelines only")
		}
	} else {
		switch v, why := r.matchRefs(only, ctx, "only"); v {
		case triFalse:
			return false, r.why(only, false, "only: does not admit it")
		case triUnknown:
			return false, why
		}
	}
	if except != nil {
		switch v, why := r.matchRefs(except, ctx, "except"); v {
		case triTrue:
			return false, r.why(except, false, "except: excludes it")
		case triUnknown:
			return false, why
		}
	}
	return true, nil
}

// matchRefs decides whether an only: or except: value matches ctx.
func (r *glReader) matchRefs(n *yaml.Node, ctx *glContext, label string) (tri, *glWhy) {
	refs := n
	if n.Kind == yaml.MappingNode {
		for _, k := range []string{"variables", "changes", "kubernetes"} {
			if key := mapKey(n, k); key != nil {
				if k == "changes" {
					return triUnknown, r.why(key, true, "%s: changes: does not count (the audit does not read which files change)", label)
				}
				return triUnknown, r.why(key, true, "%s: %s: is a condition the audit does not read", label, k)
			}
		}
		if refs = mapValue(n, "refs"); refs == nil {
			return triTrue, nil
		}
	}
	out := triFalse
	var why *glWhy
	for _, e := range listItems(refs) {
		v := matchRef(scalarOr(e, ""), ctx)
		switch {
		case v == triTrue:
			return triTrue, nil
		case v == triUnknown && why == nil:
			out, why = triUnknown, r.why(e, true, "%s: `%s` is a condition the audit does not read", label, scalarOr(e, "(not a string)"))
		}
	}
	return out, why
}

// matchRef decides one only/except ref.
func matchRef(ref string, ctx *glContext) tri {
	switch ref {
	case "branches":
		return triOf(ctx.branch != refNone)
	case "tags":
		return triFalse
	case "merge_requests":
		return triOf(ctx.source == "merge_request_event")
	case "pushes":
		return triOf(ctx.source == "push")
	case "schedules":
		return triOf(ctx.source == "schedule")
	}
	if ref == "" || strings.HasPrefix(ref, "/") || strings.Contains(ref, "@") ||
		strings.ContainsAny(ref, "*?[") || glKeywordRefs[ref] {
		return triUnknown
	}
	return triOf(ctx.branch == refDefault && ref == ctx.defaultBranch)
}

// glKeywordRefs are only/except keywords outside the closed list.
var glKeywordRefs = map[string]bool{
	"api": true, "web": true, "triggers": true, "pipelines": true, "external": true,
	"chat": true, "external_pull_requests": true,
}

// tri is a three-valued truth value.
type tri int8

const (
	triFalse tri = iota
	triTrue
	triUnknown
)

func triOf(b bool) tri {
	if b {
		return triTrue
	}
	return triFalse
}

func triAnd(a, b tri) tri {
	switch {
	case a == triFalse || b == triFalse:
		return triFalse
	case a == triTrue && b == triTrue:
		return triTrue
	}
	return triUnknown
}

func triOr(a, b tri) tri {
	switch {
	case a == triTrue || b == triTrue:
		return triTrue
	case a == triFalse && b == triFalse:
		return triFalse
	}
	return triUnknown
}

// evalExpression evaluates a rules: if: expression in ctx. One that does
// not parse is unknown.
func evalExpression(src string, ctx *glContext) tri {
	toks, ok := lexExpression(src)
	if !ok {
		return triUnknown
	}
	p := &exprParser{toks: toks, ctx: ctx}
	v, ok := p.or()
	if !ok || p.pos != len(p.toks) {
		return triUnknown
	}
	return v
}

// exprToken is one token of an expression: a variable ($NAME), a string,
// an operator, a parenthesis, or anything else (other).
type exprToken struct {
	kind string // "var", "str", "op", "(", ")", "other"
	text string
}

func lexExpression(s string) ([]exprToken, bool) {
	var out []exprToken
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '(' || c == ')':
			out = append(out, exprToken{kind: string(c)})
			i++
		case c == '$':
			j := i + 1
			brace := j < len(s) && s[j] == '{'
			if brace {
				j++
			}
			start := j
			for j < len(s) && (isNameByte(s[j])) {
				j++
			}
			name := s[start:j]
			if brace {
				if j >= len(s) || s[j] != '}' {
					return nil, false
				}
				j++
			}
			if name == "" {
				return nil, false
			}
			out = append(out, exprToken{kind: "var", text: name})
			i = j
		case c == '"' || c == '\'':
			end := strings.IndexByte(s[i+1:], c)
			if end < 0 {
				return nil, false
			}
			out = append(out, exprToken{kind: "str", text: s[i+1 : i+1+end]})
			i += end + 2
		case strings.HasPrefix(s[i:], "==") || strings.HasPrefix(s[i:], "!=") || strings.HasPrefix(s[i:], "=~") ||
			strings.HasPrefix(s[i:], "!~") || strings.HasPrefix(s[i:], "&&") || strings.HasPrefix(s[i:], "||"):
			out = append(out, exprToken{kind: "op", text: s[i : i+2]})
			i += 2
		default:
			// A regular expression, null or anything else: read to the next
			// space or parenthesis as one token the audit does not read.
			j := i
			if c == '/' {
				for j = i + 1; j < len(s) && s[j] != '/'; j++ {
					if s[j] == '\\' {
						j++
					}
				}
				j++
			}
			for j < len(s) && !strings.ContainsRune(" \t\n\r()", rune(s[j])) {
				j++
			}
			out = append(out, exprToken{kind: "other", text: s[i:min(j, len(s))]})
			i = j
		}
	}
	return out, true
}

func isNameByte(b byte) bool {
	return b == '_' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// exprParser parses or := and ('||' and)*, and := atom ('&&' atom)*,
// atom := '(' or ')' | operand [comparison operand].
type exprParser struct {
	toks []exprToken
	pos  int
	ctx  *glContext
}

func (p *exprParser) peek() (exprToken, bool) {
	if p.pos < len(p.toks) {
		return p.toks[p.pos], true
	}
	return exprToken{}, false
}

func (p *exprParser) or() (tri, bool) {
	v, ok := p.and()
	for ok {
		t, more := p.peek()
		if !more || t.kind != "op" || t.text != "||" {
			break
		}
		p.pos++
		var w tri
		if w, ok = p.and(); ok {
			v = triOr(v, w)
		}
	}
	return v, ok
}

func (p *exprParser) and() (tri, bool) {
	v, ok := p.atom()
	for ok {
		t, more := p.peek()
		if !more || t.kind != "op" || t.text != "&&" {
			break
		}
		p.pos++
		var w tri
		if w, ok = p.atom(); ok {
			v = triAnd(v, w)
		}
	}
	return v, ok
}

func (p *exprParser) atom() (tri, bool) {
	t, ok := p.peek()
	if !ok {
		return triUnknown, false
	}
	if t.kind == "(" {
		p.pos++
		v, ok := p.or()
		if c, more := p.peek(); !ok || !more || c.kind != ")" {
			return triUnknown, false
		}
		p.pos++
		return v, true
	}
	if t.kind != "var" && t.kind != "str" && t.kind != "other" {
		return triUnknown, false
	}
	p.pos++
	op, more := p.peek()
	if !more || op.kind != "op" || (op.text != "==" && op.text != "!=" && op.text != "=~" && op.text != "!~") {
		return p.presence(t), true
	}
	p.pos++
	rhs, ok := p.peek()
	if !ok || (rhs.kind != "var" && rhs.kind != "str" && rhs.kind != "other") {
		return triUnknown, false
	}
	p.pos++
	return p.compare(t, op.text, rhs), true
}

// presence decides $VAR alone: set and not empty.
func (p *exprParser) presence(t exprToken) tri {
	if t.kind != "var" {
		return triUnknown
	}
	switch t.text {
	case "CI_MERGE_REQUEST_IID":
		return triOf(p.ctx.mr)
	case "CI_COMMIT_BRANCH":
		return triOf(p.ctx.branch != refNone)
	case "CI_COMMIT_TAG":
		return triFalse
	}
	return triUnknown
}

// compare decides a comparison the closed list names, in either order.
func (p *exprParser) compare(a exprToken, op string, b exprToken) tri {
	if op != "==" && op != "!=" {
		return triUnknown
	}
	if a.kind != "var" {
		a, b = b, a
	}
	if a.kind != "var" {
		return triUnknown
	}
	eq := triUnknown
	switch {
	case a.text == "CI_PIPELINE_SOURCE" && b.kind == "str":
		eq = triOf(p.ctx.source == b.text)
	case a.text == "CI_COMMIT_BRANCH" || a.text == "CI_COMMIT_REF_NAME":
		ref := p.ctx.branch
		if a.text == "CI_COMMIT_REF_NAME" {
			ref = p.ctx.ref
		}
		switch {
		case b.kind == "var" && b.text == "CI_DEFAULT_BRANCH":
			eq = triOf(ref == refDefault)
		case b.kind == "str":
			eq = triOf(ref == refDefault && b.text == p.ctx.defaultBranch)
		}
	case a.text == "CI_DEFAULT_BRANCH" && b.kind == "var" && (b.text == "CI_COMMIT_BRANCH" || b.text == "CI_COMMIT_REF_NAME"):
		return p.compare(b, op, a)
	}
	if op == "!=" && eq != triUnknown {
		return triOf(eq == triFalse)
	}
	return eq
}
