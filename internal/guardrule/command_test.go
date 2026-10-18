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
