package contract

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"maps"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// GitLab CI as the C rules read it (spec 0055, "The GitLab pipeline
// model"). The reader resolves .gitlab-ci.yml statically, the way GitLab
// does: YAML anchors and merge keys, local includes (wildcards and include
// rules: too) merged in GitLab's order, !reference tags, extends: (deep
// merge, up to GitLab's nesting limit) and default:. Keys starting with .
// are templates. A job with trigger: include: naming local files and
// strategy: depend (or mirror) runs a child pipeline the parent waits for;
// its jobs are read with the trigger job's rules as their outer
// condition. Which jobs run for a pipeline kind is gitlab_rules.go's.
//
// Include rules can make the configuration differ by pipeline, so the
// pipeline is resolved once per evaluation context (glContext), and once
// with every include followed for the rules that read every job.

// gitlabCIFile is the GitLab CI configuration the audit reads.
const gitlabCIFile = ".gitlab-ci.yml"

// GitLab's limits, and the audit's own guards.
const (
	// gitlabExtendsDepth is how many levels of extends: GitLab resolves.
	gitlabExtendsDepth = 11
	// gitlabReferenceDepth is how deeply !reference tags may nest.
	gitlabReferenceDepth = 10
	// gitlabIncludeDepth bounds nested includes.
	gitlabIncludeDepth = 100
	// gitlabChildDepth bounds nested child pipelines.
	gitlabChildDepth = 10
	// gitlabNodeBudget bounds how far aliases may expand.
	gitlabNodeBudget = 1_000_000
)

// gitlabReserved are the top-level keys that are not jobs.
var gitlabReserved = []string{
	"default", "include", "stages", "variables", "workflow", "spec",
	"image", "services", "cache", "before_script", "after_script",
}

// gitlabDefaultKeys are the keys a job inherits from default: when it
// does not set them.
var gitlabDefaultKeys = []string{
	"after_script", "artifacts", "before_script", "cache", "hooks", "id_tokens",
	"image", "interruptible", "retry", "services", "tags", "timeout",
}

// gitlabGlobalDefaults are the deprecated top-level keys GitLab still reads
// as defaults.
var gitlabGlobalDefaults = []string{"image", "services", "cache", "before_script", "after_script"}

// glReader resolves one repository's GitLab CI.
type glReader struct {
	env *Env
	// origin is the file every node read came from.
	origin map[*yaml.Node]string
	docs   map[string]*yaml.Node
	loaded map[string]bool
	broken []Finding
	seen   map[string]bool
	// includes are the unfollowed includes, recorded while every include
	// is followed.
	includes     []CIInclude
	seenIncludes map[*yaml.Node]bool
	nodes        int
	pipelines    map[string]*glPipeline
	// dropped maps a context's key to the files include rules left out
	// of its pipeline, each with the rule's reason.
	dropped map[string]map[string]*glWhy
}

// glPipeline is one pipeline (the main one or a child) resolved for one
// context.
type glPipeline struct {
	child string
	root  *yaml.Node
	jobs  []*glJob
}

// glJob is one job of a resolved pipeline.
type glJob struct {
	ci   *CIJob
	node *yaml.Node
	// child is the child pipeline's files when the job triggers one the
	// parent waits for.
	child []string
}

// readGitLabCI reads .gitlab-ci.yml into the CI model.
func readGitLabCI(env *Env) *CI {
	r := &glReader{env: env, origin: map[*yaml.Node]string{}, docs: map[string]*yaml.Node{},
		loaded: map[string]bool{}, seen: map[string]bool{}, seenIncludes: map[*yaml.Node]bool{},
		pipelines: map[string]*glPipeline{}, dropped: map[string]map[string]*glWhy{}}
	ci := &CI{Provider: CIGitLab, Dir: gitlabCIFile}

	all := &glContext{all: true}
	r.walk(r.pipeline([]string{gitlabCIFile}, "", all, nil), all, nil, 0, nil, func(j *CIJob, _ bool, _ *glWhy) {
		ci.Jobs = append(ci.Jobs, j)
	})
	ci.Includes = r.includes

	branch := gitlabDefaultBranch(env, r)
	ci.runs = map[PipelineKind]*CIRun{
		KindMergeRequest: r.run(KindMergeRequest, branch, ci.Jobs),
		KindDefaultPush:  r.run(KindDefaultPush, branch, ci.Jobs),
	}
	ci.Broken = r.broken
	return ci
}

// fail records a file that cannot be resolved; each failure is kept once.
func (r *glReader) fail(file string, err error) {
	f := ParseFailure(file, err)
	if key := f.File + "\x00" + f.Message; !r.seen[key] {
		r.seen[key] = true
		r.broken = append(r.broken, f)
	}
}

// failAt records a failure at a node, in the file the node came from.
func (r *glReader) failAt(n *yaml.Node, msg string) {
	r.fail(r.origin[n], lineError(n, msg))
}

// load parses and expands one file once. It returns nil when the file
// cannot be read; the failure is recorded.
func (r *glReader) load(file string) *yaml.Node {
	if r.loaded[file] {
		return r.docs[file]
	}
	r.loaded[file] = true
	data, err := r.env.ReadFile(file)
	var root *yaml.Node
	if err == nil {
		root, err = r.parse(file, data)
	}
	if err != nil {
		r.fail(file, err)
		return nil
	}
	r.docs[file] = root
	return root
}

// parse reads a file's configuration document: the first, or the second
// when the first is a spec: header.
func (r *glReader) parse(file string, data []byte) (*yaml.Node, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var docs []*yaml.Node
	for {
		var doc yaml.Node
		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		docs = append(docs, &doc)
	}
	if len(docs) == 0 || len(docs[0].Content) == 0 {
		return nil, errors.New("the file is empty")
	}
	top := resolve(docs[0].Content[0])
	if len(docs) > 1 && mapValue(top, "spec") != nil && len(docs[1].Content) > 0 {
		top = resolve(docs[1].Content[0])
	}
	if top.Kind != yaml.MappingNode {
		return nil, errors.New("a GitLab CI configuration must be a map")
	}
	return r.expand(file, top, 0)
}

// expand copies a node with aliases followed and merge keys (<<) applied:
// a key written in the map wins over a merged one, and an earlier merged
// map over a later one.
func (r *glReader) expand(file string, n *yaml.Node, depth int) (*yaml.Node, error) {
	r.nodes++
	if r.nodes > gitlabNodeBudget || depth > 1000 {
		return nil, errors.New("its aliases expand to too large a configuration")
	}
	switch n.Kind {
	case yaml.AliasNode:
		return r.expand(file, n.Alias, depth+1)
	case yaml.SequenceNode:
		out := r.like(file, n)
		for _, c := range n.Content {
			e, err := r.expand(file, c, depth+1)
			if err != nil {
				return nil, err
			}
			out.Content = append(out.Content, e)
		}
		return out, nil
	case yaml.MappingNode:
		explicit := map[string]bool{}
		for i := 0; i+1 < len(n.Content); i += 2 {
			if !isMergeKey(n.Content[i]) {
				explicit[n.Content[i].Value] = true
			}
		}
		out := r.like(file, n)
		added := map[string]bool{}
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			if !isMergeKey(k) {
				e, err := r.expand(file, v, depth+1)
				if err != nil {
					return nil, err
				}
				r.origin[k] = file
				out.Content = append(out.Content, k, e)
				added[k.Value] = true
				continue
			}
			sources := []*yaml.Node{v}
			if rv := resolve(v); rv.Kind == yaml.SequenceNode {
				sources = rv.Content
			}
			for _, s := range sources {
				m, err := r.expand(file, s, depth+1)
				if err != nil {
					return nil, err
				}
				if m.Kind != yaml.MappingNode {
					return nil, lineError(v, "a merge key (<<) must name a map")
				}
				for j := 0; j+1 < len(m.Content); j += 2 {
					if key := m.Content[j].Value; !explicit[key] && !added[key] {
						added[key] = true
						out.Content = append(out.Content, m.Content[j], m.Content[j+1])
					}
				}
			}
		}
		return out, nil
	default:
		r.origin[n] = file
		return n, nil
	}
}

func isMergeKey(k *yaml.Node) bool {
	return k.Kind == yaml.ScalarNode && (k.Tag == "!!merge" || (k.Value == "<<" && k.Style == 0))
}

// like returns an empty node of n's kind, tag and position, from file.
func (r *glReader) like(file string, n *yaml.Node) *yaml.Node {
	out := &yaml.Node{Kind: n.Kind, Tag: n.Tag, Style: n.Style, Line: n.Line, Column: n.Column}
	r.origin[out] = file
	return out
}

// deepMerge merges b over a: maps key by key, recursively; any other value
// of b replaces a's. Either may be nil. Neither is changed.
func (r *glReader) deepMerge(a, b *yaml.Node) *yaml.Node {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	case a.Kind != yaml.MappingNode || b.Kind != yaml.MappingNode:
		return b
	}
	out := r.like(r.origin[b], b)
	inB := map[string]int{}
	for i := 0; i+1 < len(b.Content); i += 2 {
		inB[b.Content[i].Value] = i
	}
	done := map[string]bool{}
	for i := 0; i+1 < len(a.Content); i += 2 {
		key := a.Content[i].Value
		if j, ok := inB[key]; ok {
			out.Content = append(out.Content, b.Content[j], r.deepMerge(a.Content[i+1], b.Content[j+1]))
			done[key] = true
			continue
		}
		out.Content = append(out.Content, a.Content[i], a.Content[i+1])
	}
	for i := 0; i+1 < len(b.Content); i += 2 {
		if !done[b.Content[i].Value] {
			out.Content = append(out.Content, b.Content[i], b.Content[i+1])
		}
	}
	return out
}

// without returns a map without key.
func (r *glReader) without(m *yaml.Node, key string) *yaml.Node {
	out := r.like(r.origin[m], m)
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value != key {
			out.Content = append(out.Content, m.Content[i], m.Content[i+1])
		}
	}
	return out
}

// mapKey returns the key node of key in a map, or nil.
func mapKey(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i]
		}
	}
	return nil
}

// mergeFile returns a file's configuration with its includes merged in
// GitLab's order: each include in turn, then the file itself over them.
func (r *glReader) mergeFile(file string, ctx *glContext, stack []string) *yaml.Node {
	if slices.Contains(stack, file) {
		r.fail(stack[0], fmt.Errorf("include cycle: %s", strings.Join(append(stack, file), " includes ")))
		return nil
	}
	if len(stack) > gitlabIncludeDepth {
		r.fail(stack[0], fmt.Errorf("includes nest more than %d levels", gitlabIncludeDepth))
		return nil
	}
	root := r.load(file)
	if root == nil {
		return nil
	}
	var out *yaml.Node
	stack = append(stack, file)
	if inc := mapValue(root, "include"); inc != nil {
		for _, e := range listItems(inc) {
			out = r.deepMerge(out, r.include(file, e, ctx, stack))
		}
	}
	return r.deepMerge(out, r.without(root, "include"))
}

// listItems returns a list's items, or the node itself for any other
// value.
func listItems(n *yaml.Node) []*yaml.Node {
	if n.Kind == yaml.SequenceNode {
		return n.Content
	}
	return []*yaml.Node{n}
}

// glIncludeKinds are the include keys GitLab reads, local first.
var glIncludeKinds = []string{"local", "project", "remote", "template", "component", "artifact"}

// includeKind classifies an include entry: its kind and, for a local one,
// the path.
func includeKind(e *yaml.Node) (kind, local string) {
	switch e.Kind {
	case yaml.ScalarNode:
		if strings.HasPrefix(e.Value, "https://") || strings.HasPrefix(e.Value, "http://") {
			return "remote", ""
		}
		return "local", e.Value
	case yaml.MappingNode:
		for _, k := range glIncludeKinds {
			if v := mapValue(e, k); v != nil {
				if k == "local" {
					return k, scalarOr(v, "")
				}
				return k, ""
			}
		}
	}
	return "", ""
}

// include follows one include entry of file: a local one whose rules, if
// any, admit ctx. Any other kind is recorded, not followed.
func (r *glReader) include(file string, e *yaml.Node, ctx *glContext, stack []string) *yaml.Node {
	kind, local := includeKind(e)
	switch kind {
	case "":
		r.failAt(e, "an include must be a path or a map with local:, project:, remote:, template:, component: or artifact:")
		return nil
	case "local":
	default:
		r.record(file, kind, e)
		return nil
	}
	files := r.localFiles(local)
	if rules := mapValue(e, "rules"); rules != nil && !ctx.all {
		if ok, why := r.evalRules(rules, ctx, "include rules"); !ok {
			r.dropInclude(ctx, files, why)
			return nil
		}
	}
	if len(files) == 0 {
		r.failAt(e, "include local: "+local+" names no tracked file")
		return nil
	}
	var out *yaml.Node
	for _, f := range files {
		out = r.deepMerge(out, r.mergeFile(f, ctx, stack))
	}
	return out
}

// dropInclude records, for ctx, the files an include whose rules do not
// admit ctx would have brought in, local includes of theirs too, with
// why: a job of those files that runs nowhere is reported with it.
func (r *glReader) dropInclude(ctx *glContext, files []string, why *glWhy) {
	k := ctx.key()
	if r.dropped[k] == nil {
		r.dropped[k] = map[string]*glWhy{}
	}
	var visit func(f string)
	visit = func(f string) {
		if _, ok := r.dropped[k][f]; ok {
			return
		}
		r.dropped[k][f] = why
		root := r.load(f)
		if root == nil {
			return
		}
		if inc := mapValue(root, "include"); inc != nil {
			for _, e := range listItems(inc) {
				if kind, local := includeKind(e); kind == "local" {
					for _, g := range r.localFiles(local) {
						visit(g)
					}
				}
			}
		}
	}
	for _, f := range files {
		visit(f)
	}
}

// droppedWhy returns why a job is in none of the pipelines of contexts:
// an include whose rules leave its file out, preferring one the audit
// could not read. It returns nil when no include left it out.
func (r *glReader) droppedWhy(j *CIJob, contexts []*glContext) *glWhy {
	var out *glWhy
	for _, ctx := range contexts {
		for _, c := range []*glContext{ctx, ctx.child()} {
			why := r.dropped[c.key()][j.File]
			if why != nil && (out == nil || (why.unread && !out.unread)) {
				out = why
			}
		}
	}
	return out
}

// record keeps an unfollowed include once.
func (r *glReader) record(file, kind string, e *yaml.Node) {
	if r.seenIncludes[e] {
		return
	}
	r.seenIncludes[e] = true
	r.includes = append(r.includes, CIInclude{File: file, Line: e.Line, Kind: kind, Node: e})
}

// localFiles returns the tracked files a local include names: one path,
// or GitLab's wildcards (* within a directory, ** across directories).
func (r *glReader) localFiles(p string) []string {
	p = strings.TrimPrefix(p, "/")
	if !strings.Contains(p, "*") {
		if r.env.Repo.HasFile(p) {
			return []string{p}
		}
		return nil
	}
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(p); i++ {
		switch {
		case strings.HasPrefix(p[i:], "**"):
			b.WriteString(".*")
			i++
		case p[i] == '*':
			b.WriteString("[^/]*")
		default:
			b.WriteString(regexp.QuoteMeta(p[i : i+1]))
		}
	}
	b.WriteString("$")
	re := regexp.MustCompile(b.String())
	var out []string
	for _, f := range r.env.Repo.Files() {
		if re.MatchString(f) {
			out = append(out, f)
		}
	}
	return out
}

// pipeline resolves the pipeline of files (the main one, or a child's)
// for ctx, once.
func (r *glReader) pipeline(files []string, child string, ctx *glContext, stack []string) *glPipeline {
	key := child + "\x00" + ctx.key()
	if p := r.pipelines[key]; p != nil {
		return p
	}
	p := &glPipeline{child: child}
	r.pipelines[key] = p
	var merged *yaml.Node
	for _, f := range files {
		merged = r.deepMerge(merged, r.mergeFile(f, ctx, stack))
	}
	if merged == nil {
		return p
	}
	root := r.references(merged, merged, nil)
	p.root = root
	def := r.defaults(root)
	extended := map[string]*yaml.Node{}
	for i := 0; i+1 < len(root.Content); i += 2 {
		k, v := root.Content[i], root.Content[i+1]
		name := k.Value
		if slices.Contains(gitlabReserved, name) || strings.HasPrefix(name, ".") {
			continue
		}
		if v.Kind != yaml.MappingNode {
			r.failAt(k, "job "+name+" must be a map")
			continue
		}
		node := r.applyDefault(r.extended(root, name, extended, nil), def)
		p.jobs = append(p.jobs, r.readJob(k, name, child, node, ctx))
	}
	return p
}

// references replaces every !reference tag under n with the value it
// points at in root, resolved in turn.
func (r *glReader) references(root, n *yaml.Node, stack []string) *yaml.Node {
	switch n.Kind {
	case yaml.SequenceNode:
		if n.Tag == "!reference" {
			return r.reference(root, n, stack)
		}
		out := r.like(r.origin[n], n)
		for _, c := range n.Content {
			out.Content = append(out.Content, r.references(root, c, stack))
		}
		return out
	case yaml.MappingNode:
		out := r.like(r.origin[n], n)
		for i := 0; i+1 < len(n.Content); i += 2 {
			out.Content = append(out.Content, n.Content[i], r.references(root, n.Content[i+1], stack))
		}
		return out
	}
	return n
}

// reference resolves one !reference tag. One that points at nothing, is
// part of a cycle or nests too deeply fails; it resolves to an empty list.
func (r *glReader) reference(root, n *yaml.Node, stack []string) *yaml.Node {
	empty := r.like(r.origin[n], &yaml.Node{Kind: yaml.SequenceNode, Line: n.Line, Column: n.Column})
	var path []string
	for _, c := range n.Content {
		if c.Kind != yaml.ScalarNode {
			r.failAt(n, "a !reference must be a list of names")
			return empty
		}
		path = append(path, c.Value)
	}
	name := "!reference [" + strings.Join(path, ", ") + "]"
	switch {
	case len(path) == 0:
		r.failAt(n, "a !reference must name a key")
		return empty
	case slices.Contains(stack, name):
		r.failAt(n, name+" refers to itself")
		return empty
	case len(stack) >= gitlabReferenceDepth:
		r.failAt(n, fmt.Sprintf("!reference tags nest more than %d levels", gitlabReferenceDepth))
		return empty
	}
	cur := root
	for _, p := range path {
		if cur = mapValue(cur, p); cur == nil {
			r.failAt(n, name+" points at nothing")
			return empty
		}
	}
	return r.references(root, cur, append(stack, name))
}

// extended returns a job or template with its extends: applied: each
// parent in turn, then the job itself over them.
func (r *glReader) extended(root *yaml.Node, name string, memo map[string]*yaml.Node, stack []string) *yaml.Node {
	if n, ok := memo[name]; ok {
		return n
	}
	node := mapValue(root, name)
	ext := mapValue(node, "extends")
	if ext == nil {
		memo[name] = node
		return node
	}
	own := r.without(node, "extends")
	if slices.Contains(stack, name) {
		r.failAt(ext, "extends cycle: "+strings.Join(append(stack, name), " extends "))
		return own
	}
	if len(stack) >= gitlabExtendsDepth {
		r.failAt(ext, fmt.Sprintf("extends nests more than %d levels", gitlabExtendsDepth))
		return own
	}
	var out *yaml.Node
	for _, pn := range listItems(ext) {
		if pn.Kind != yaml.ScalarNode {
			r.failAt(pn, "job "+name+": extends must be a name or a list of names")
			continue
		}
		parent := mapValue(root, pn.Value)
		if parent == nil || parent.Kind != yaml.MappingNode {
			r.failAt(pn, "job "+name+" extends "+pn.Value+", which is not defined")
			continue
		}
		out = r.deepMerge(out, r.extended(root, pn.Value, memo, append(stack, name)))
	}
	out = r.deepMerge(out, own)
	memo[name] = out
	return out
}

// defaults returns the keys jobs inherit: default:, with the deprecated
// top-level keys where default: does not set them.
func (r *glReader) defaults(root *yaml.Node) *yaml.Node {
	def := &yaml.Node{Kind: yaml.MappingNode}
	if d := mapValue(root, "default"); d != nil && d.Kind == yaml.MappingNode {
		def.Content = append(def.Content, d.Content...)
	}
	for _, k := range gitlabGlobalDefaults {
		if mapValue(def, k) == nil {
			if key := mapKey(root, k); key != nil {
				def.Content = append(def.Content, key, mapValue(root, k))
			}
		}
	}
	return def
}

// applyDefault adds the default keys a job inherits and does not set,
// as inherit: default: allows.
func (r *glReader) applyDefault(job, def *yaml.Node) *yaml.Node {
	allowed := gitlabDefaultKeys
	if d := mapValue(mapValue(job, "inherit"), "default"); d != nil {
		switch {
		case d.Kind == yaml.ScalarNode && d.Value == "false":
			return job
		case d.Kind == yaml.SequenceNode:
			allowed = nil
			for _, c := range d.Content {
				allowed = append(allowed, c.Value)
			}
		}
	}
	out := r.like(r.origin[job], job)
	out.Content = append(out.Content, job.Content...)
	for _, k := range allowed {
		if mapValue(job, k) == nil && slices.Contains(gitlabDefaultKeys, k) {
			if key := mapKey(def, k); key != nil {
				out.Content = append(out.Content, key, mapValue(def, k))
			}
		}
	}
	return out
}

// glScriptKeys are a job's scripts in the order they run; after_script
// does not count as calling verify: its failure does not fail the job.
var glScriptKeys = []struct {
	key    string
	counts bool
}{{"before_script", true}, {"script", true}, {"after_script", false}}

// readJob reads a resolved job: its scripts, and the child pipeline it
// triggers when the parent waits for it.
func (r *glReader) readJob(k *yaml.Node, name, child string, node *yaml.Node, ctx *glContext) *glJob {
	cj := &CIJob{File: r.origin[k], ID: name, Line: k.Line, Child: child, Node: node, key: gitlabJobKey(child, name)}
	for _, s := range glScriptKeys {
		v := mapValue(node, s.key)
		if v == nil {
			continue
		}
		items, ok := scriptLines(v, 0)
		if !ok {
			r.failAt(v, "job "+name+": "+s.key+" must be a string or a list of strings")
			continue
		}
		for _, it := range items {
			line := it.Line
			if it.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0 {
				line++
			}
			sc := CIScript{File: r.origin[it], Line: line, Where: "job " + name + ", " + s.key, Counts: s.counts,
				Script: ParseShellScript(it.Value)}
			cj.Scripts = append(cj.Scripts, sc)
			if s.counts {
				cj.Commands = append(cj.Commands, sc.Commands...)
			}
		}
	}
	cj.Images = r.images(node)
	return &glJob{ci: cj, node: node, child: r.childFiles(cj.File, name, node, ctx)}
}

// images reads a resolved job's image: and services: entries: each a
// string or a map with name:.
func (r *glReader) images(node *yaml.Node) []CIImage {
	var out []CIImage
	for _, key := range []string{"image", "services"} {
		v := mapValue(node, key)
		if v == nil {
			continue
		}
		entries := []*yaml.Node{v}
		if key == "services" {
			entries = listItems(v)
		}
		for _, e := range entries {
			n := e
			if e.Kind == yaml.MappingNode {
				if n = mapValue(e, "name"); n == nil {
					n = e
				}
			}
			out = append(out, CIImage{File: r.origin[n], Line: n.Line, Key: key, Name: scalarOr(n, "")})
		}
	}
	return out
}

// scriptLines flattens a script: one string, or a list of strings and
// nested lists (as !reference produces), up to GitLab's ten levels.
func scriptLines(n *yaml.Node, depth int) ([]*yaml.Node, bool) {
	switch {
	case n.Kind == yaml.ScalarNode:
		return []*yaml.Node{n}, true
	case n.Kind == yaml.SequenceNode && depth < 10:
		var out []*yaml.Node
		for _, c := range n.Content {
			items, ok := scriptLines(c, depth+1)
			if !ok {
				return nil, false
			}
			out = append(out, items...)
		}
		return out, true
	}
	return nil, false
}

// childFiles returns the local files of a child pipeline the job
// triggers and the parent waits for (strategy: depend, or mirror). Other
// trigger includes are recorded, not followed.
func (r *glReader) childFiles(file, name string, node *yaml.Node, ctx *glContext) []string {
	t := mapValue(node, "trigger")
	if t == nil || t.Kind != yaml.MappingNode {
		return nil
	}
	inc := mapValue(t, "include")
	if inc == nil {
		return nil
	}
	strategy := scalarOr(mapValue(t, "strategy"), "")
	waits := strategy == "depend" || strategy == "mirror"
	var files []string
	for _, e := range listItems(inc) {
		kind, local := includeKind(e)
		switch kind {
		case "local":
			if !waits {
				continue
			}
			found := r.localFiles(local)
			if len(found) == 0 {
				r.failAt(e, "job "+name+": trigger include "+local+" names no tracked file")
			}
			files = append(files, found...)
		case "":
			r.failAt(e, "job "+name+": a trigger include must be a path or a map with local:, project:, template:, component: or artifact:")
		default:
			if ctx.all {
				r.record(file, kind, e)
			}
		}
	}
	return files
}

// walk visits the jobs of a pipeline and of the child pipelines it waits
// for, in order, with whether each runs in ctx and, when not, why. outer
// is the trigger job's reason for not running, for a child pipeline's
// jobs. In the every-job context nothing is evaluated.
func (r *glReader) walk(p *glPipeline, ctx *glContext, outer *glWhy, depth int, stack []string,
	visit func(j *CIJob, runs bool, why *glWhy)) {
	for _, j := range p.jobs {
		runs, why := outer == nil, outer
		if runs && !ctx.all {
			runs, why = r.admitted(p, j, ctx)
		}
		if j.child == nil {
			visit(j.ci, runs, why)
			continue
		}
		name := strings.Join(j.child, ", ")
		if slices.Contains(stack, name) || depth >= gitlabChildDepth {
			r.failAt(mapValue(j.node, "trigger"), "job "+j.ci.ID+": child pipelines nest in a cycle or too deeply")
			continue
		}
		var childOuter *glWhy
		if !runs {
			w := *why
			w.text = "its trigger job " + j.ci.ID + " does not run: " + why.text
			childOuter = &w
		}
		cctx := ctx.child()
		r.walk(r.pipeline(j.child, name, cctx, nil), cctx, childOuter, depth+1, append(stack, name), visit)
	}
}

// run decides which jobs run for a pipeline kind, on branch as the
// default branch: a job runs when it runs in any of the kind's contexts.
// A job that runs in none is skipped with the reason of the first context
// whose condition the audit could not read, or else of the first context.
// A job of every that is in none of the kind's pipelines because
// include rules left its file out is skipped with those rules' reason.
func (r *glReader) run(kind PipelineKind, branch string, every []*CIJob) *CIRun {
	type entry struct {
		job  *CIJob
		runs bool
		why  *glWhy
	}
	var order []string
	got := map[string]*entry{}
	contexts := contextsFor(kind, branch)
	for _, ctx := range contexts {
		r.walk(r.pipeline([]string{gitlabCIFile}, "", ctx, nil), ctx, nil, 0, nil, func(j *CIJob, runs bool, why *glWhy) {
			e := got[j.Key()]
			if e == nil {
				e = &entry{job: j, runs: runs, why: why}
				got[j.Key()] = e
				order = append(order, j.Key())
				return
			}
			switch {
			case e.runs:
			case runs:
				e.job, e.runs, e.why = j, true, nil
			case why != nil && why.unread && (e.why == nil || !e.why.unread):
				e.why = why
			}
		})
	}
	out := &CIRun{}
	for _, k := range order {
		e := got[k]
		if e.runs {
			out.Jobs = append(out.Jobs, e.job)
			continue
		}
		out.Skipped = append(out.Skipped, CISkip{Job: e.job, File: e.why.file, Line: e.why.line, Why: e.why.text})
	}
	for _, j := range every {
		if got[j.Key()] != nil {
			continue
		}
		if why := r.droppedWhy(j, contexts); why != nil {
			out.Skipped = append(out.Skipped, CISkip{Job: j, File: why.file, Line: why.line, Why: why.text})
		}
	}
	return out
}

// gitlabDefaultBranch decides the default branch for GitLab CI: the
// target of refs/remotes/origin/HEAD, or else main or master, the one the
// configuration's conditions name more often (rules: if: literals,
// only/except refs), main on a tie. In that case the report says which it
// assumed.
func gitlabDefaultBranch(env *Env, r *glReader) string {
	if env.defaultBranch != "" {
		return env.defaultBranch
	}
	if b := env.Repo.originHead; b != "" {
		env.defaultBranch = b
		return b
	}
	named := map[string]int{}
	for _, f := range slices.Sorted(maps.Keys(r.docs)) {
		countBranchNames(r.docs[f], named)
	}
	branch, best := fallbackBranches[0], -1
	for _, b := range fallbackBranches {
		if named[b] > best {
			branch, best = b, named[b]
		}
	}
	noteAssumedBranch(env, branch)
	env.defaultBranch = branch
	return branch
}

// countBranchNames counts the literals named in if: conditions and the
// refs named in only: and except: under n.
func countBranchNames(n *yaml.Node, named map[string]int) {
	switch n.Kind {
	case yaml.SequenceNode:
		for _, c := range n.Content {
			countBranchNames(c, named)
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i].Value, n.Content[i+1]
			switch {
			case k == "if" && v.Kind == yaml.ScalarNode:
				toks, _ := lexExpression(v.Value)
				for _, t := range toks {
					if t.kind == "str" {
						named[t.text]++
					}
				}
			case k == "only" || k == "except":
				refs := v
				if v.Kind == yaml.MappingNode {
					if refs = mapValue(v, "refs"); refs == nil {
						continue
					}
				}
				for _, e := range listItems(refs) {
					named[scalarOr(e, "")]++
				}
			default:
				countBranchNames(v, named)
			}
		}
	}
}
