package claude

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
)

// Field names the part of a tool call a rule inspects. The guard maps a
// tool call's tool_name to one of these through Policy.Tools and matches
// the rule's Pattern against tool_input[field].
type Field string

const (
	// FieldCommand is the shell command of a Bash or PowerShell call.
	FieldCommand Field = "command"
	// FieldFilePath is the target file of a Write or Edit call.
	FieldFilePath Field = "file_path"
)

// Rule is one thing the guard denies.
//
// Pattern is matched against the parsed field. Raw is matched against the
// whole stdin payload when it cannot be parsed, or names a tool or field
// the policy doesn't know. Raw is the pattern the bash hooks used before
// spec 0021, which is what keeps the fallback exactly as strict as they
// were. For every command rule the two are the same; the file rule's Raw
// matches the JSON text around the path, because that is all the old hook
// could see.
type Rule struct {
	ID      string `json:"id"`
	Field   Field  `json:"field"`
	Pattern string `json:"pattern"`
	Raw     string `json:"raw"`
	Message string `json:"message"`
}

// Policy is the whole guard configuration, serialized into
// .claude/hooks/policy.json for the per-runtime guards to interpret. See
// docs/specs/0021-agent-hooks-task-interface.md.
type Policy struct {
	// Version is the policy.json layout, bumped only if an interpreter would
	// misread the new shape. Version 2 adds Exceptions (spec 0052).
	Version int `json:"version"`
	// Tools maps each field to the tool_name values whose calls carry it.
	// MultiEdit is listed because the settings route its calls to the guard
	// too, and the bash hook the guard replaced checked them.
	Tools map[Field][]string `json:"tools"`
	Rules []Rule             `json:"rules"`
	// Exceptions are the contract's guard exceptions, which rule A-04 lists
	// (spec 0051). VibeConform generates none, and the guards refuse every
	// call while the list has anything in it, until #105 makes them apply
	// it. A repository's own exceptions go in policy.local.json (spec 0050).
	Exceptions []Exception `json:"exceptions"`
}

// Exception lifts the guard rule Rule for the values Match matches in
// full, for Reason.
type Exception struct {
	Rule   string `json:"rule"`
	Match  string `json:"match"`
	Reason string `json:"reason"`
}

// commandRule builds a command rule, whose parsed and raw patterns are
// the same.
func commandRule(id, pattern, message string) Rule {
	return Rule{ID: id, Field: FieldCommand, Pattern: pattern, Raw: pattern, Message: message}
}

// The parts the command patterns are built from (spec 0047). Each must stay
// in the subset Go, Python and JavaScript read alike (spec 0021 §3).
const (
	// cmdStart is where a command word may begin, so /bin/rm, sudo rm and
	// $(rm match while farm and rm-old do not.
	cmdStart = `(^|[^a-zA-Z0-9_.-])`
	// word is one word of a simple command. It never holds a separator, so
	// no match crosses into the next command.
	word = `[^ \t;&|\n]+`
	// gap is up to 16 whole words of the same command. The cap keeps the
	// backtracking engines linear: with any text allowed between two
	// options, a long commit message took Python over a minute.
	gap = `(` + word + `[ \t]+){0,16}`
	// optEnd closes an option that another option follows. The colon
	// covers PowerShell's -Recurse:$true.
	optEnd = `(:[^ \t;&|\n]*)?[ \t]+`
	// lastEnd closes the last option.
	lastEnd = `([^a-zA-Z0-9_-]|$)`
	// fullTree is a pathspec for the whole working tree.
	fullTree = `["']?(\.|\./|:/)["']?`
)

// both matches cmd with options a and b, in either order.
func both(cmd, a, b string) string {
	return cmd + gap + a + optEnd + gap + b + lastEnd + "|" + cmd + gap + b + optEnd + gap + a + lastEnd
}

// prefixes matches opt or any prefix of it at least minLen bytes long:
// the abbreviations of a long option that the tool accepts.
func prefixes(opt string, minLen int) string {
	tail := ""
	for i := len(opt) - 1; i >= minLen; i-- {
		tail = "(" + regexp.QuoteMeta(opt[i:i+1]) + tail + ")?"
	}
	return regexp.QuoteMeta(opt[:minLen]) + tail
}

// anyCase matches s in any letter case. The engines share no portable
// case-insensitive flag, and PowerShell ignores case.
func anyCase(s string) string {
	var b strings.Builder
	for _, r := range s {
		if lower, upper := strings.ToLower(string(r)), strings.ToUpper(string(r)); lower != upper {
			b.WriteString("[" + upper + lower + "]")
			continue
		}
		b.WriteString(regexp.QuoteMeta(string(r)))
	}
	return b.String()
}

// anyCasePrefixes is prefixes for a PowerShell parameter: any case.
func anyCasePrefixes(opt string, minLen int) string {
	tail := ""
	for i := len(opt) - 1; i >= minLen; i-- {
		tail = "(" + anyCase(opt[i:i+1]) + tail + ")?"
	}
	return anyCase(opt[:minLen]) + tail
}

// gitCommand matches git, up to eight of its global options, and the
// subcommand sub.
func gitCommand(sub string) string {
	globalOpt := `-[Cc][ \t]+` + word + `|--(git-dir|work-tree|namespace|exec-path|super-prefix|config-env)[ \t]+` + word + `|-[^ \t;&|\n]*`
	return cmdStart + `git([ \t]+(` + globalOpt + `)){0,8}[ \t]+` + sub + `[ \t]+`
}

// Command patterns. The comments name the spellings each one covers; the
// corpus (testdata/guard_corpus.json) holds the examples.
var (
	// rm with recursion (-r, -R, --recursive or a prefix) and force (-f,
	// --force or a prefix), separate in either order or in one cluster.
	rmRF = func() string {
		cmd := cmdStart + `rm[ \t]+`
		recursive := `(-[a-zA-Z]*[rR][a-zA-Z]*|` + prefixes("--recursive", 3) + `)`
		force := `(-[a-zA-Z]*f[a-zA-Z]*|` + prefixes("--force", 3) + `)`
		return both(cmd, recursive, force) + "|" + cmd + gap + `-[a-zA-Z]*([rR][a-zA-Z]*f|f[a-zA-Z]*[rR])`
	}()

	// Remove-Item or an alias of it, in any case, with -Recurse (-r and up)
	// and -Force (-fo and up: -f alone is ambiguous with -Filter).
	removeItemRF = func() string {
		names := []string{"Remove-Item", "ri", "del", "erase", "rd", "rmdir", "rm"}
		for i, n := range names {
			names[i] = anyCase(n)
		}
		cmd := cmdStart + `(` + strings.Join(names, "|") + `)[ \t]+`
		return both(cmd, `-`+anyCasePrefixes("recurse", 1), `-`+anyCasePrefixes("force", 2))
	}()

	// git reset --hard, or --ha / --har.
	gitResetHard = gitCommand("reset") + gap + prefixes("--hard", 4) + lastEnd

	// git clean with force (-f, --force or a prefix) and -d.
	gitCleanFD = func() string {
		force := `(-[a-zA-Z]*f[a-zA-Z]*|` + prefixes("--force", 3) + `)`
		return both(gitCommand("clean"), force, `-[a-zA-Z]*d[a-zA-Z]*`) + "|" +
			gitCommand("clean") + gap + `-[a-zA-Z]*(f[a-zA-Z]*d|d[a-zA-Z]*f)`
	}()

	// git push with -f in a cluster, exactly --force (Git rejects --forc as
	// ambiguous), or a refspec forced with +. --force-with-lease is not
	// --force.
	gitPushForce = gitCommand("push") + gap + `(-[a-zA-Z]*f[a-zA-Z]*` + lastEnd + `|--force` + lastEnd + `|["']?\+[^ \t])`

	// git branch -D, or delete (-d, --delete or a prefix) with force (-f,
	// --forc or --force: --for is ambiguous with --format).
	gitBranchForceDelete = func() string {
		del := `(-[a-zA-Z]*d[a-zA-Z]*|` + prefixes("--delete", 3) + `)`
		force := `(-[a-zA-Z]*f[a-zA-Z]*|` + prefixes("--force", 6) + `)`
		return gitCommand("branch") + gap + `-[a-zA-Z]*D` + "|" + both(gitCommand("branch"), del, force) + "|" +
			gitCommand("branch") + gap + `-[a-zA-Z]*(d[a-zA-Z]*f|f[a-zA-Z]*d)`
	}()

	// git checkout or git restore of the whole working tree. git restore
	// --staged (-S) without --worktree (-W) only unstages, so it is
	// allowed: the first alternative accepts any word before the pathspec
	// except a --staged or -S one.
	gitDiscardWorktree = func() string {
		notStaged := `([^- \t;&|\n][^ \t;&|\n]*|--|--[^s \t;&|\n][^ \t;&|\n]*|--s[^t \t;&|\n][^ \t;&|\n]*|-[a-zA-RT-Z0-9]+)[ \t]+`
		worktree := `(-[a-zA-Z]*W[a-zA-Z]*|` + prefixes("--worktree", 3) + `)`
		treeEnd := `([ \t;&|)]|$)`
		return gitCommand("restore") + `(` + notStaged + `){0,16}` + fullTree + treeEnd + "|" +
			both(gitCommand("restore"), worktree, fullTree) + "|" +
			gitCommand("checkout") + gap + fullTree + treeEnd
	}()
)

// escape ends every command deny message: the agent asks, the user runs it.
// A command run with ! in Claude Code does not go through the guard (spec
// 0047, verified).
const escape = " Ask the user to run it themselves (in Claude Code: ! <command>)."

// policy is the single source of the guard's rules. It began as a port, rule
// for rule, of block-dangerous.sh and block-secret-files.sh (spec 0021).
// Spec 0047 widened the command rules to every spelling the tools accept for
// the same command, and added git-discard-worktree.
//
// Changing what is blocked is a policy decision, not a refactor: keep it
// out of commits that only move code (spec 0021, non-goals).
var policy = Policy{
	Version:    2,
	Exceptions: []Exception{},
	Tools: map[Field][]string{
		FieldCommand:  {"Bash", "PowerShell"},
		FieldFilePath: {"Edit", "MultiEdit", "Write"},
	},
	Rules: []Rule{
		commandRule("rm-rf", rmRF,
			"Blocked: rm -rf (or equivalent) is a destructive operation."+escape),
		commandRule("remove-item-recurse-force", removeItemRF,
			"Blocked: Remove-Item -Recurse -Force (or equivalent) is a destructive operation."+escape),
		commandRule("git-reset-hard", gitResetHard,
			"Blocked: git reset --hard discards uncommitted work."+escape),
		commandRule("git-clean-fd", gitCleanFD,
			"Blocked: git clean -fd (or equivalent) permanently deletes untracked files."+escape),
		commandRule("git-push-force", gitPushForce,
			"Blocked: git push --force (or a +refspec) can overwrite remote history."+escape),
		commandRule("git-branch-force-delete", gitBranchForceDelete,
			"Blocked: git branch -D (or equivalent) force-deletes a branch."+escape),
		commandRule("git-discard-worktree", gitDiscardWorktree,
			"Blocked: git restore . (or git checkout -- .) discards uncommitted changes to tracked files."+escape),
		{
			ID:      "secret-file",
			Field:   FieldFilePath,
			Pattern: `(\.env(\.[a-zA-Z]+)?|\.pem|\.key|id_rsa|id_ed25519|credentials\.json|\.npmrc|\.netrc)$`,
			Raw:     `"file_path"\s*:\s*"[^"]*(\.env(\.[a-zA-Z]+)?|\.pem|\.key|id_rsa|id_ed25519|credentials\.json|\.npmrc|\.netrc)"`,
			Message: "Blocked: this looks like a secret/credential file. Ask the user to edit it themselves (in Claude Code: ! <command>, or their editor).",
		},
		{
			// An agent the guard refuses must not write itself an exception
			// (spec 0050 D12). The guard is not a sandbox: a shell write
			// still gets through.
			ID:      "guard-policy-file",
			Field:   FieldFilePath,
			Pattern: `(^|[/\\])\.claude[/\\]+hooks[/\\]+policy(\.local)?\.json$`,
			Raw:     `"file_path"\s*:\s*"([^"]*[/\\])?\.claude[/\\]+hooks[/\\]+policy(\.local)?\.json"`,
			Message: "Blocked: the guard's policy files are the user's to change, an exception above all. Ask the user to edit it themselves (in Claude Code: ! <command>, or their editor).",
		},
	},
}

// renderPolicy serializes policy as .claude/hooks/policy.json content:
// two-space indent, LF, trailing newline, and no HTML escaping, so the
// patterns read in the file exactly as they are written above. Output is
// deterministic because the only map is keyed by Field, which encoding/json
// sorts.
func renderPolicy(p Policy) ([]byte, error) {
	for _, tools := range p.Tools {
		if !slices.IsSorted(tools) {
			return nil, fmt.Errorf("render policy: tool list %v is not sorted", tools)
		}
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(p); err != nil {
		return nil, fmt.Errorf("render policy: %w", err)
	}
	return collapseStringArrays(buf.Bytes()), nil
}

// multilineStringArray matches an indented JSON array whose elements are
// all plain strings with no escapes, one per line, as json.Encoder writes
// them.
var multilineStringArray = regexp.MustCompile(`(?m)^( *)("[^"\\\n]*": )\[\n((?: *"[^"\\\n]*",?\n)+) *\]`)

// printWidth is the widest line collapseStringArrays will produce:
// Prettier's default print width, below prod-ts's configured 100, so the
// result is stable under either.
const printWidth = 80

// collapseStringArrays puts each short array of plain strings on one line,
// which is how Prettier formats JSON. prod-ts's fmt:check runs Prettier
// over **/*.json, .claude/ included, so a policy.json in json.Encoder's
// one-element-per-line style would fail every adopter's format check.
// Arrays that would not fit stay expanded, as Prettier leaves them.
func collapseStringArrays(b []byte) []byte {
	return multilineStringArray.ReplaceAllFunc(b, func(m []byte) []byte {
		parts := multilineStringArray.FindSubmatch(m)
		indent, key, body := parts[1], parts[2], parts[3]

		var elems []string
		for line := range strings.SplitSeq(strings.TrimSpace(string(body)), "\n") {
			elems = append(elems, strings.TrimSuffix(strings.TrimSpace(line), ","))
		}
		oneLine := string(indent) + string(key) + "[" + strings.Join(elems, ", ") + "]"
		if len(oneLine) > printWidth {
			return m
		}
		return []byte(oneLine)
	})
}

// LocalPolicyPath is the repository's own guard policy (spec 0050): extra
// rules and exceptions the guards read next to policy.json. VibeConform
// never writes it.
const LocalPolicyPath = ".claude/hooks/policy.local.json"

// localCheck is audit's reading of a policy.local.json.
type localCheck struct {
	// Broken is why the guards would deny every call with this file, or "".
	Broken string
	// Problems are what else audit reports as a conflict: entries the
	// guards accept but that cannot do what they say.
	Problems []string
	// Notes list each exception and extra rule, for review.
	Notes []string
}

// checkLocal reads a policy.local.json as the guards do (their addLocal),
// and adds the checks only audit makes, against the managed policy p.
func checkLocal(p Policy, content []byte) localCheck {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(content, &top); err != nil || top == nil {
		return localCheck{Broken: "not a JSON object"}
	}
	for _, key := range slices.Sorted(maps.Keys(top)) {
		if key != "version" && key != "rules" && key != "exceptions" {
			return localCheck{Broken: fmt.Sprintf("unknown key %q; only version, rules and exceptions are allowed", key)}
		}
	}
	var version int
	if err := json.Unmarshal(top["version"], &version); err != nil || version != 1 {
		return localCheck{Broken: "version is not 1, the only version the guards understand"}
	}
	rules, err := localObjects(top, "rules")
	if err != nil {
		return localCheck{Broken: err.Error()}
	}
	exceptions, err := localObjects(top, "exceptions")
	if err != nil {
		return localCheck{Broken: err.Error()}
	}

	var c localCheck
	ids := map[string]bool{}
	for _, r := range p.Rules {
		ids[r.ID] = true
	}
	for i, r := range rules {
		for _, key := range []string{"id", "field", "pattern", "message"} {
			if v, ok := r[key].(string); !ok || v == "" {
				return localCheck{Broken: fmt.Sprintf("rule %d has no %s", i+1, key)}
			}
		}
		id, field, pattern := r["id"].(string), r["field"].(string), r["pattern"].(string)
		if _, ok := p.Tools[Field(field)]; !ok {
			return localCheck{Broken: fmt.Sprintf("rule %s has field %q, which the policy does not know", id, field)}
		}
		if ids[id] {
			c.Problems = append(c.Problems, fmt.Sprintf("rule id %s is already taken; exceptions for it would be ambiguous", id))
		}
		ids[id] = true
		patterns := []string{pattern}
		if raw, ok := r["raw"].(string); ok && raw != "" {
			patterns = append(patterns, raw)
		}
		for _, pat := range patterns {
			if _, err := regexp.Compile(pat); err != nil {
				c.Problems = append(c.Problems, fmt.Sprintf("rule %s: pattern %q does not compile, so it denies every %s", id, pat, field))
			}
		}
		c.Notes = append(c.Notes, fmt.Sprintf("extra rule %s: %s", id, pattern))
	}
	for i, e := range exceptions {
		id, _ := e["rule"].(string)
		pattern, _ := e["pattern"].(string)
		if id == "" || pattern == "" {
			return localCheck{Broken: fmt.Sprintf("exception %d needs a rule and a pattern", i+1)}
		}
		if !ids[id] {
			c.Problems = append(c.Problems, fmt.Sprintf("exception for %s: no rule has that id, so it lifts nothing", id))
		}
		if _, err := regexp.Compile(pattern); err != nil {
			c.Problems = append(c.Problems, fmt.Sprintf("exception for %s: pattern %q does not compile, so it lifts nothing", id, pattern))
		}
		note := fmt.Sprintf("exception %s: %s", id, pattern)
		if reason, ok := e["reason"].(string); ok && reason != "" {
			note += " (" + reason + ")"
		}
		c.Notes = append(c.Notes, note)
	}
	return c
}

// localObjects decodes top[key], when present, as a list of objects.
func localObjects(top map[string]json.RawMessage, key string) ([]map[string]any, error) {
	raw, ok := top[key]
	if !ok {
		return nil, nil
	}
	var list []map[string]any
	if err := json.Unmarshal(raw, &list); err != nil || list == nil || slices.ContainsFunc(list, func(m map[string]any) bool { return m == nil }) {
		return nil, fmt.Errorf("%s is not a list of objects", key)
	}
	return list, nil
}
