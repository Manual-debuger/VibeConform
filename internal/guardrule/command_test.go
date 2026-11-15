package guardrule_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/guardrule"
)

// show writes the commands found in src, one per entry, as their words
// joined by spaces. A word that holds an unquoted expansion is written
// as <?>, a quoted expansion inside a word as <$>. Redirection targets
// follow " >" and the path a command was found through follows " @".
func show(t *testing.T, shell guardrule.Shell, src string) []string {
	t.Helper()
	cmds, err := guardrule.Commands(shell, src)
	if err != nil {
		t.Fatalf("Commands(%q): %v", src, err)
	}
	var out []string
	for _, c := range cmds {
		var parts []string
		for _, w := range c.Words {
			parts = append(parts, showWord(w))
		}
		s := strings.Join(parts, " ")
		for _, r := range c.Redirects {
			s += " >" + showWord(r)
		}
		if len(c.Via) > 0 {
			s += " @" + strings.Join(c.Via, ",")
		}
		out = append(out, s)
	}
	return out
}

func showWord(w guardrule.Word) string {
	if w.Unknown {
		return "<?>"
	}
	return strings.ReplaceAll(w.Text, guardrule.ExpansionMark, "<$>")
}

func expect(t *testing.T, shell guardrule.Shell, src string, want ...string) {
	t.Helper()
	got := show(t, shell, src)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("Commands(%q)\n got: %q\nwant: %q", src, got, want)
	}
}

func TestBashSimpleCommandsEverywhere(t *testing.T) {
	cases := []struct {
		src  string
		want []string
	}{
		{"rm -rf build", []string{"rm -rf build"}},
		{"cd /tmp && rm -rf", []string{"cd /tmp", "rm -rf"}},
		{"a; b || c | d", []string{"a", "b", "c", "d"}},
		{"(rm -r -f x)", []string{"rm -r -f x"}},
		{"{ rm -rf x; }", []string{"rm -rf x"}},
		{"if true; then rm -rf x; fi", []string{"true", "rm -rf x"}},
		{"for f in a; do rm -rf $f; done", []string{"rm -rf <?>"}},
		{"while false; do git clean -fd; done", []string{"false", "git clean -fd"}},
		{"f() { git reset --hard; }; f", []string{"git reset --hard", "f"}},
		{"echo $(git reset --hard)", []string{"echo <?>", "git reset --hard"}},
		{"echo `git reset --hard`", []string{"echo <?>", "git reset --hard"}},
		{"diff <(git reset --hard) x", []string{"diff <?> x", "git reset --hard"}},
		{"x=$(rm -rf y)", []string{"rm -rf y"}},
		{"time rm -rf x", []string{"rm -rf x"}},
		{"! rm -rf x", []string{"rm -rf x"}},
		{"rm -rf x &", []string{"rm -rf x"}},
		{"FOO=1 rm -rf x", []string{"rm -rf x"}},
		{"rm -r build\ntouch -f x", []string{"rm -r build", "touch -f x"}},
		{"[[ -f x ]] && rm -rf y", []string{"rm -rf y"}},
	}
	for _, c := range cases {
		expect(t, guardrule.ShellBash, c.src, c.want...)
	}
}

func TestBashQuoting(t *testing.T) {
	cases := []struct{ src, want string }{
		{`"rm" -rf x`, "rm -rf x"},
		{`r\m -rf x`, "rm -rf x"},
		{`r''m -rf x`, "rm -rf x"},
		{`rm "-rf" x`, "rm -rf x"},
		{`git restore "."`, "git restore ."},
		{`rm $'-\x72f' x`, "rm -rf x"},
		{`rm $'\055rf' x`, "rm -rf x"},
		{`rm -r "a;b" -f`, "rm -r a;b -f"},
		{`rm -r "$dir" -f`, "rm -r <$> -f"},
		{`rm -r $dir`, "rm -r <?>"},
		{`rm -r x$dir`, "rm -r <?>"},
		{`rm {-r,-f} x`, "rm <?> x"},
		{`echo $"hi"`, "echo hi"},
	}
	for _, c := range cases {
		expect(t, guardrule.ShellBash, c.src, c.want)
	}
}

func TestBashWordKinds(t *testing.T) {
	cmds, err := guardrule.Commands(guardrule.ShellBash, `rm lit "q" "$a" $b`)
	if err != nil {
		t.Fatal(err)
	}
	w := cmds[0].Words
	if !w[1].Literal || w[1].Quoted || w[1].Unknown {
		t.Errorf("lit = %+v", w[1])
	}
	if !w[2].Literal || !w[2].Quoted {
		t.Errorf("quoted literal = %+v", w[2])
	}
	if w[3].Literal || w[3].Unknown || !w[3].Quoted {
		t.Errorf("quoted expansion is an operand: %+v", w[3])
	}
	if w[4].Literal || !w[4].Unknown {
		t.Errorf("unquoted expansion is unknown: %+v", w[4])
	}
}

func TestRedirectTargets(t *testing.T) {
	expect(t, guardrule.ShellBash, "echo x > .git/vibe-unlock", "echo x >.git/vibe-unlock")
	expect(t, guardrule.ShellBash, "{ echo x; } >> f", " >f", "echo x")
	expect(t, guardrule.ShellBash, "> f", " >f")
	expect(t, guardrule.ShellPowerShell, "Get-Date > .git/vibe-unlock", "Get-Date >.git/vibe-unlock")
	expect(t, guardrule.ShellPowerShell, "Get-Date 2>&1 | Out-File x", "Get-Date", "Out-File x")
	// An expression's output redirected to a file writes the file too:
	// a redirect-only command, as in Bash.
	expect(t, guardrule.ShellPowerShell, `'x' > .git\vibe-unlock`, ` >.git\vibe-unlock`)
	expect(t, guardrule.ShellPowerShell, `$t >> f; 1 *> g`, ` >f`, ` >g`)
	expect(t, guardrule.ShellPowerShell, `(Get-Date) 2>&1`, `Get-Date`)
}

func TestWrappersAreStripped(t *testing.T) {
	cases := []struct {
		src  string
		want []string
	}{
		{"sudo rm -rf x", []string{"rm -rf x @sudo"}},
		{"sudo -u root -E rm -rf x", []string{"rm -rf x @sudo"}},
		{"env A=1 B=2 rm -rf x", []string{"rm -rf x @env"}},
		{"env -i -u HOME rm -rf x", []string{"rm -rf x @env"}},
		{"env -S 'rm -rf x'", []string{"env -S rm -rf x", "rm -rf x @env -S"}},
		{"command rm -rf x", []string{"rm -rf x @command"}},
		{"command -v rm", []string{"command -v rm"}},
		{"builtin rm -rf x", []string{"rm -rf x @builtin"}},
		{"exec rm -rf x", []string{"rm -rf x @exec"}},
		{"nohup rm -rf x", []string{"rm -rf x @nohup"}},
		{"/usr/bin/time -v rm -rf x", []string{"rm -rf x @time"}},
		{"nice -n 5 rm -rf x", []string{"rm -rf x @nice"}},
		{"sudo nohup env A=1 rm -rf x", []string{"rm -rf x @sudo,nohup,env"}},
		{"xargs rm -r", []string{"rm -r <?> @xargs"}},
		{"xargs -0 -n 1 rm -r", []string{"rm -r <?> @xargs"}},
		{"xargs -I{} rm -r {} -f", []string{"rm -r <?> -f @xargs"}},
		{"find . -name x -exec rm -rf {} \\;", []string{"find . -name x -exec rm -rf {} ;", "rm -rf {} @find -exec"}},
		{"find . -execdir rm -rf {} + -ok git clean -fd \\;", []string{
			"find . -execdir rm -rf {} + -ok git clean -fd ;", "rm -rf {} @find -execdir", "git clean -fd @find -ok"}},
		{"sudo", []string{"sudo"}},
	}
	for _, c := range cases {
		expect(t, guardrule.ShellBash, c.src, c.want...)
	}
}

// A wrapper reads a long option as getopt_long does (spec 0062): a
// unique prefix is that option, so it takes the option's argument.
func TestWrapperLongOptionAbbreviations(t *testing.T) {
	cases := []struct{ full, short, want string }{
		{"sudo --user root rm -rf x", "sudo --us root rm -rf x", "rm -rf x @sudo"},
		{"sudo --user=root rm -rf x", "sudo --u=root rm -rf x", "rm -rf x @sudo"},
		{"env --unset X rm -rf x", "env --u X rm -rf x", "rm -rf x @env"},
		{"env --chdir /tmp rm -rf x", "env --ch /tmp rm -rf x", "rm -rf x @env"},
		{"timeout --signal KILL 5 rm -rf x", "timeout --sig KILL 5 rm -rf x", "rm -rf x @timeout"},
		{"timeout --kill-after 1 5 rm -rf x", "timeout --k 1 5 rm -rf x", "rm -rf x @timeout"},
		{"nice --adjustment 5 rm -rf x", "nice --adj 5 rm -rf x", "rm -rf x @nice"},
		{"stdbuf --output L rm -rf x", "stdbuf --o L rm -rf x", "rm -rf x @stdbuf"},
		{"stdbuf --input 0 rm -rf x", "stdbuf --in 0 rm -rf x", "rm -rf x @stdbuf"},
		{"command time --output f rm -rf x", "command time --o f rm -rf x", "rm -rf x @command,time"},
		{"/usr/bin/time --format %e rm -rf x", "/usr/bin/time --fo %e rm -rf x", "rm -rf x @time"},
		{"xargs --arg-file f rm -rf", "xargs --arg f rm -rf", "rm -rf <?> @xargs"},
		{"xargs --max-args 1 rm -rf", "xargs --max-a 1 rm -rf", "rm -rf <?> @xargs"},
		{"xargs --delimiter , rm -rf", "xargs --deli , rm -rf", "rm -rf <?> @xargs"},
		{"xargs --replace=F rm -rf F", "xargs --rep=F rm -rf F", "rm -rf <?> @xargs"},
	}
	for _, c := range cases {
		expect(t, guardrule.ShellBash, c.full, c.want)
		expect(t, guardrule.ShellBash, c.short, c.want)
	}
}

// A long option the guard does not know, or a prefix shared by options
// that differ in whether they take an argument, is read both ways
// (spec 0062).
func TestWrapperUnknownLongOptionIsReadBothWays(t *testing.T) {
	cases := []struct {
		src  string
		want []string
	}{
		{"sudo --newopt root rm -rf x", []string{"root rm -rf x @sudo", "rm -rf x @sudo"}},
		{"sudo --newopt ls", []string{"ls @sudo"}},
		{"sudo --newopt=x rm -rf y", []string{"rm -rf y @sudo"}},
		{"env --newopt X rm -rf x", []string{"X rm -rf x @env", "rm -rf x @env"}},
		{"nohup --newopt rm -rf x", []string{"rm -rf x @nohup", "x @nohup"}},
		// --max- is max-lines (optional argument), max-args, max-chars and
		// max-procs (a required one).
		{"xargs --max- 1 rm -rf", []string{"1 rm -rf <?> @xargs", "rm -rf <?> @xargs"}},
		{"sudo --zz --yy rm -rf x", []string{"rm -rf x @sudo", "x @sudo"}},
		// Too many readings: what runs is not known.
		{"sudo" + strings.Repeat(" --zz", 12) + " ls", []string{"<?> @sudo"}},
	}
	for _, c := range cases {
		expect(t, guardrule.ShellBash, c.src, c.want...)
	}
}

// env's split string is found under every spelling env accepts.
func TestEnvSplitStringSpellings(t *testing.T) {
	for _, src := range []string{
		"env -S 'rm -rf x'", "env -S'rm -rf x'", "env -iS 'rm -rf x'",
		"env --split-string 'rm -rf x'", "env --split-string='rm -rf x'",
		"env --sp 'rm -rf x'", "env --sp='rm -rf x'", "env -u X -S 'rm -rf x'",
	} {
		got := show(t, guardrule.ShellBash, src)
		if len(got) != 2 || got[1] != "rm -rf x @env -S" {
			t.Errorf("Commands(%q) = %q, want env, then rm -rf x @env -S", src, got)
		}
	}
}

// env reads its split string's words as its own arguments again, as
// GNU env does: options, then assignments, then the command it runs.
func TestEnvSplitStringIsReadAsEnvArguments(t *testing.T) {
	cases := []struct {
		src  string
		want []string
	}{
		{"env -S '-i rm -rf x'", []string{"env -S -i rm -rf x", "rm -rf x @env -S"}},
		{"env -S '-u X rm -rf x'", []string{"env -S -u X rm -rf x", "rm -rf x @env -S"}},
		{"env -S '--unset=X -- rm -rf x'", []string{"env -S --unset=X -- rm -rf x", "rm -rf x @env -S"}},
		{"env -S 'A=1 rm -rf x'", []string{"env -S A=1 rm -rf x", "rm -rf x @env -S"}},
		{"env -S '-i' rm -rf x", []string{"env -S -i rm -rf x", "rm -rf x @env -S"}},
		{"env -S '-S \"rm -rf x\"'", []string{"env -S -S \"rm -rf x\"", "env -S rm -rf x @env -S", "rm -rf x @env -S,env -S"}},
		{"env -S 'ls -l'", []string{"env -S ls -l", "ls -l @env -S"}},
		{"env -S '-i'", []string{"env -S -i", "env -i @env -S"}},
	}
	for _, c := range cases {
		expect(t, guardrule.ShellBash, c.src, c.want...)
	}
}

func TestNestedShellsAreParsedAgain(t *testing.T) {
	cases := []struct {
		shell guardrule.Shell
		src   string
		want  []string
	}{
		{guardrule.ShellBash, "bash -c 'rm -rf x'", []string{"bash -c rm -rf x", "rm -rf x @bash -c"}},
		{guardrule.ShellBash, "sh -c \"git reset --hard\"", []string{"sh -c git reset --hard", "git reset --hard @sh -c"}},
		{guardrule.ShellBash, "bash -lc 'rm -rf x'", []string{"bash -lc rm -rf x", "rm -rf x @bash -c"}},
		{guardrule.ShellBash, "zsh -e -c 'rm -rf x' extra", []string{"zsh -e -c rm -rf x extra", "rm -rf x @zsh -c"}},
		// -o takes the next word, and the cluster goes on: -o posix -c.
		{guardrule.ShellBash, "bash -oc posix 'rm -rf x'", []string{"bash -oc posix rm -rf x", "rm -rf x @bash -c"}},
		{guardrule.ShellBash, "bash -O extglob +o posix -c 'rm -rf x'", []string{
			"bash -O extglob +o posix -c rm -rf x", "rm -rf x @bash -c"}},
		{guardrule.ShellBash, "bash - -c", []string{"bash - -c"}},
		{guardrule.ShellBash, "eval 'rm -rf' x", []string{"eval rm -rf x", "rm -rf x @eval"}},
		{guardrule.ShellBash, "eval \"rm -rf $d\"", []string{"eval rm -rf <$>", "rm -rf <?> @eval"}},
		{guardrule.ShellBash, "eval $cmd", []string{"eval <?>", "<?> @eval"}},
		{guardrule.ShellBash, "bash -c \"$x\"", []string{"bash -c <$>", "<?> @bash -c"}},
		{guardrule.ShellBash, "bash <<'EOF'\nrm -rf x\nEOF", []string{"bash", "rm -rf x @bash <<"}},
		{guardrule.ShellBash, "sh <<< 'git clean -fd'", []string{"sh", "git clean -fd @sh <<"}},
		{guardrule.ShellBash, "pwsh -c 'ri -Rec -Fo x'", []string{"pwsh -c ri -Rec -Fo x", "ri -Rec -Fo x @pwsh -c"}},
		{guardrule.ShellBash, "powershell.exe -Command \"Remove-Item x -r -fo\"", []string{
			"powershell.exe -Command Remove-Item x -r -fo", "Remove-Item x -r -fo @powershell -c"}},
		{guardrule.ShellPowerShell, "bash -c 'rm -rf x'", []string{"bash -c rm -rf x", "rm -rf x @bash -c"}},
		{guardrule.ShellPowerShell, "pwsh -NoProfile -Command 'Remove-Item -r -fo x'", []string{
			"pwsh -NoProfile -Command Remove-Item -r -fo x", "Remove-Item -r -fo x @pwsh -c"}},
		{guardrule.ShellPowerShell, "Invoke-Expression 'git reset --hard'", []string{
			"Invoke-Expression git reset --hard", "git reset --hard @iex"}},
		{guardrule.ShellPowerShell, "iex \"rm -r -fo x\"", []string{"iex rm -r -fo x", "rm -r -fo x @iex"}},
		{guardrule.ShellBash, "sudo bash -c 'sh -c \"rm -rf x\"'", []string{
			"bash -c sh -c \"rm -rf x\" @sudo", "sh -c rm -rf x @sudo,bash -c", "rm -rf x @sudo,bash -c,sh -c"}},
	}
	for _, c := range cases {
		expect(t, c.shell, c.src, c.want...)
	}
}

func TestShellHereDocumentAfterDashS(t *testing.T) {
	expect(t, guardrule.ShellBash, "bash -s x <<'EOF'\nrm -rf y\nEOF", "bash -s x", "rm -rf y @bash <<")
	expect(t, guardrule.ShellBash, "sh -s -- a b <<< 'git clean -fd'", "sh -s -- a b", "git clean -fd @sh <<")
	// The here-document feeds the script file, not the shell's script.
	expect(t, guardrule.ShellBash, "bash x.sh <<'EOF'\nrm -rf y\nEOF", "bash x.sh")
	// -c wins over -s: the here-document is the string's input.
	expect(t, guardrule.ShellBash, "bash -s -c ls <<'EOF'\nrm -rf y\nEOF", "bash -s -c ls", "ls @bash -c")
}

func TestShellScriptSource(t *testing.T) {
	cases := []struct {
		src  string
		want guardrule.ScriptSource
	}{
		{"ls -c x", guardrule.ScriptNone},
		{"bash -c 'ls'", guardrule.ScriptFromString},
		{"bash -ec 'ls' name arg", guardrule.ScriptFromString},
		{"bash +c 'ls'", guardrule.ScriptFromString},
		{"bash -c", guardrule.ScriptFromString},
		{"bash script.sh", guardrule.ScriptFromFile},
		{"bash -e script.sh arg", guardrule.ScriptFromFile},
		{"bash -- script.sh", guardrule.ScriptFromFile},
		{"bash \"$f\"", guardrule.ScriptFromFile},
		{"bash", guardrule.ScriptFromStdin},
		{"bash -", guardrule.ScriptFromStdin},
		{"bash -e -x", guardrule.ScriptFromStdin},
		{"bash -s", guardrule.ScriptFromStdin},
		{"bash -s foo bar", guardrule.ScriptFromStdin},
		{"bash -es foo", guardrule.ScriptFromStdin},
		{"bash -s x < x.sh", guardrule.ScriptFromStdin},
		{"bash -o posix", guardrule.ScriptFromStdin},
		{"bash +o posix", guardrule.ScriptFromStdin},
		{"bash -O extglob", guardrule.ScriptFromStdin},
		{"bash +O extglob", guardrule.ScriptFromStdin},
		{"bash -eo pipefail", guardrule.ScriptFromStdin},
		{"bash --rcfile y", guardrule.ScriptFromStdin},
		{"bash --init-file y", guardrule.ScriptFromStdin},
		{"bash --norc --rcfile y -o posix x.sh", guardrule.ScriptFromFile},
		{"bash -o posix -c 'ls'", guardrule.ScriptFromString},
		{"bash <<'EOF'\nls\nEOF", guardrule.ScriptFromHereDoc},
		{"bash -s x <<'EOF'\nls\nEOF", guardrule.ScriptFromHereDoc},
		{"sh -s x <<< 'ls'", guardrule.ScriptFromHereDoc},
		{"bash --version", guardrule.ScriptNone},
		{"bash --help", guardrule.ScriptNone},
		{"zsh -o", guardrule.ScriptFromStdin},
		{"/usr/bin/dash -s", guardrule.ScriptFromStdin},
	}
	for _, c := range cases {
		cmds, err := guardrule.Commands(guardrule.ShellBash, c.src)
		if err != nil {
			t.Fatalf("Commands(%q): %v", c.src, err)
		}
		if got := cmds[0].ScriptSource; got != c.want {
			t.Errorf("Commands(%q)[0].ScriptSource = %v, want %v", c.src, got, c.want)
		}
	}
}

func TestShellScriptSourceThroughWrappers(t *testing.T) {
	cmds, err := guardrule.Commands(guardrule.ShellBash, "curl https://x | sudo -u root bash -s foo")
	if err != nil {
		t.Fatal(err)
	}
	last := cmds[len(cmds)-1]
	if last.Name() != "bash" || last.ScriptSource != guardrule.ScriptFromStdin {
		t.Errorf("last command %q has ScriptSource %v, want bash reading stdin", last.String(), last.ScriptSource)
	}
}

func TestNestedCommandShell(t *testing.T) {
	cmds, err := guardrule.Commands(guardrule.ShellBash, "pwsh -c 'ri -Rec -Fo x'")
	if err != nil {
		t.Fatal(err)
	}
	if cmds[1].Shell != guardrule.ShellPowerShell || cmds[1].Depth != 1 {
		t.Errorf("nested command = %+v", cmds[1])
	}
	if cmds[0].Shell != guardrule.ShellBash || cmds[0].Depth != 0 {
		t.Errorf("outer command = %+v", cmds[0])
	}
}

func TestNestingDepth(t *testing.T) {
	src := "rm -rf x"
	for range 8 {
		src = "bash -c " + bashQuote(src)
	}
	if _, err := guardrule.Commands(guardrule.ShellBash, src); err != nil {
		t.Fatalf("depth 8: %v", err)
	}
	src = "bash -c " + bashQuote(src)
	_, err := guardrule.Commands(guardrule.ShellBash, src)
	var pe *guardrule.ParseError
	if !errors.As(err, &pe) || !strings.Contains(err.Error(), "deeper than 8") {
		t.Fatalf("depth 9: err = %v", err)
	}
}

func bashQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func TestParseErrorsFailClosed(t *testing.T) {
	cases := []struct {
		shell guardrule.Shell
		src   string
	}{
		{guardrule.ShellBash, "rm 'x"},
		{guardrule.ShellBash, "if true; then"},
		{guardrule.ShellBash, "bash -c 'rm \"x'"},
		{guardrule.ShellPowerShell, "Remove-Item 'x"},
		{guardrule.ShellPowerShell, "Remove-Item \"x"},
		{guardrule.ShellPowerShell, "Get-ChildItem | ForEach-Object { Remove-Item $_"},
		{guardrule.ShellPowerShell, "Write-Output $(Get-Date"},
		{guardrule.ShellPowerShell, "<# open comment"},
		{guardrule.ShellPowerShell, "Get-Content x < y"},
		{guardrule.ShellPowerShell, "@'\nno end"},
		{guardrule.ShellBash, "pwsh -c 'Remove-Item \"x'"},
	}
	for _, c := range cases {
		_, err := guardrule.Commands(c.shell, c.src)
		var pe *guardrule.ParseError
		if !errors.As(err, &pe) {
			t.Errorf("Commands(%q) err = %v, want a ParseError", c.src, err)
		}
	}
}

func TestPowerShellTokenizing(t *testing.T) {
	ps := guardrule.ShellPowerShell
	cases := []struct {
		src  string
		want []string
	}{
		{"Remove-Item -Recurse -Force build", []string{"Remove-Item -Recurse -Force build"}},
		{`Remove-Item "build dir" -Recurse -Force`, []string{"Remove-Item build dir -Recurse -Force"}},
		{`Remove-Item 'it''s' -r`, []string{"Remove-Item it's -r"}},
		{"Remove-Item `\"a b`\" -r", []string{`Remove-Item "a b" -r`}},
		{"Remove-Item a`\n -r", []string{"Remove-Item a -r"}},
		{"Remove-Item -Force -Recurse:$true build", []string{"Remove-Item -Force -Recurse:$true build"}},
		{"Get-ChildItem dist | Remove-Item -Recurse -Force", []string{"Get-ChildItem dist", "Remove-Item -Recurse -Force"}},
		{"Remove-Item a.txt -Force; Get-ChildItem -Recurse", []string{"Remove-Item a.txt -Force", "Get-ChildItem -Recurse"}},
		{"a && b || c\nd", []string{"a", "b", "c", "d"}},
		{"& 'Remove-Item' -r -fo x", []string{"Remove-Item -r -fo x"}},
		{"& $cmd -r -fo x", []string{"<?> -r -fo x"}},
		{". ./setup.ps1", []string{"./setup.ps1"}},
		{"Write-Output $(Remove-Item -r -fo x)", []string{"Write-Output <?>", "Remove-Item -r -fo x"}},
		{`Write-Output "now: $(git reset --hard)"`, []string{"Write-Output now: <$>", "git reset --hard"}},
		{"Get-ChildItem | ForEach-Object { Remove-Item $_ -r -fo }", []string{"Get-ChildItem", "ForEach-Object <$>", "Remove-Item <?> -r -fo"}},
		{"$files = Get-ChildItem; $files | Remove-Item -r -fo", []string{"Get-ChildItem", "Remove-Item -r -fo"}},
		{"if (Test-Path x) { Remove-Item x -r -fo } else { git clean -fd }", []string{"Test-Path x", "Remove-Item x -r -fo", "git clean -fd"}},
		{"foreach ($f in $list) { rd $f -r -fo }", []string{"rd <?> -r -fo"}},
		{"function Clean { ri -Rec -Fo build }; Clean", []string{"ri -Rec -Fo build", "Clean"}},
		{"Remove-Item x -r # -fo", []string{"Remove-Item x -r"}},
		{"<# Remove-Item -r -fo #> Get-Date", []string{"Get-Date"}},
		{"Remove-Item @args", []string{"Remove-Item <?>"}},
		{"$h = @{ rm = '-rf' }", nil},
		{"Set-Content x @'\nrm -rf\n'@", []string{"Set-Content x rm -rf"}},
		{"git --% push --force", []string{"git push --force"}},
		{"return Remove-Item x -r -fo", []string{"Remove-Item x -r -fo"}},
		{"Remove-Item a,b -r -fo", []string{"Remove-Item a b -r -fo"}},
		{"Start-Job -ScriptBlock { git reset --hard }", []string{"Start-Job -ScriptBlock <$>", "git reset --hard"}},
		{"(Get-Item x).Delete()", []string{"Get-Item x"}},
	}
	for _, c := range cases {
		expect(t, ps, c.src, c.want...)
	}
}

func TestPowerShellWordKinds(t *testing.T) {
	cmds, err := guardrule.Commands(guardrule.ShellPowerShell, `Remove-Item '-Recurse' $x "$y" -Recurse:$true`)
	if err != nil {
		t.Fatal(err)
	}
	w := cmds[0].Words
	if !w[1].Quoted || !w[1].Literal {
		t.Errorf("quoted parameter-like word = %+v", w[1])
	}
	if !w[2].Unknown {
		t.Errorf("unquoted variable = %+v", w[2])
	}
	if w[3].Unknown || w[3].Literal {
		t.Errorf("quoted variable = %+v", w[3])
	}
	if !w[4].Literal {
		t.Errorf("-Recurse:$true is literal: %+v", w[4])
	}
}

func TestCommandNames(t *testing.T) {
	cases := []struct {
		shell        guardrule.Shell
		src          string
		name, cmdlet string
		dynamic      bool
	}{
		{guardrule.ShellBash, "/bin/rm -rf x", "rm", "", false},
		{guardrule.ShellBash, "RM.EXE -rf x", "rm", "", false},
		{guardrule.ShellBash, `'C:\tools\git.exe' push`, "git", "", false},
		{guardrule.ShellBash, "$c -rf build", "", "", true},
		{guardrule.ShellBash, `"$c" -rf build`, "", "", true},
		{guardrule.ShellPowerShell, "ri -Rec x", "ri", "remove-item", false},
		{guardrule.ShellPowerShell, "ERASE x", "erase", "remove-item", false},
		{guardrule.ShellPowerShell, "Microsoft.PowerShell.Management\\Remove-Item x", "remove-item", "remove-item", false},
		{guardrule.ShellPowerShell, "rm.exe -rf x", "rm", "", false},
		{guardrule.ShellPowerShell, "Get-ChildItem", "get-childitem", "get-childitem", false},
		{guardrule.ShellPowerShell, "& $c -r", "", "", true},
	}
	for _, c := range cases {
		cmds, err := guardrule.Commands(c.shell, c.src)
		if err != nil {
			t.Fatal(err)
		}
		got := cmds[0]
		if got.Name() != c.name || got.Cmdlet() != c.cmdlet || got.Dynamic() != c.dynamic {
			t.Errorf("%q: Name %q Cmdlet %q Dynamic %v, want %q %q %v",
				c.src, got.Name(), got.Cmdlet(), got.Dynamic(), c.name, c.cmdlet, c.dynamic)
		}
	}
}

func TestPowerShellParameters(t *testing.T) {
	cases := []struct {
		word, param string
		ok          bool
		value       string
	}{
		{"-Recurse", "Recurse", true, ""},
		{"-rec", "Recurse", true, ""},
		{"-R", "Recurse", true, ""},
		{"-Recurse:$true", "Recurse", true, "$true"},
		{"-Recurse:$false", "Recurse", true, "$false"},
		{"-Recursive", "Recurse", false, ""},
		{"-", "Recurse", false, ""},
		{"Recurse", "Recurse", false, ""},
		{"-FO", "Force", true, ""},
	}
	for _, c := range cases {
		w := guardrule.Word{Text: c.word, Literal: true}
		ok, value := guardrule.PSParam(w, c.param)
		if ok != c.ok || value != c.value {
			t.Errorf("PSParam(%q, %q) = %v %q, want %v %q", c.word, c.param, ok, value, c.ok, c.value)
		}
	}
	if ok, _ := guardrule.PSParam(guardrule.Word{Text: "-Recurse", Literal: true, Quoted: true}, "Recurse"); ok {
		t.Error("a quoted word is not a parameter")
	}
}
