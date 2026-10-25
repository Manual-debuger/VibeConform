package guardrule_test

import (
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/guardrule"
)

// writes lists the paths every simple command of src writes, in order,
// each written as showWord writes it.
func writes(t *testing.T, shell guardrule.Shell, src string) []string {
	t.Helper()
	cmds, err := guardrule.Commands(shell, src)
	if err != nil {
		t.Fatalf("Commands(%q): %v", src, err)
	}
	var out []string
	for _, c := range cmds {
		for _, w := range c.Writes {
			out = append(out, showWord(w))
		}
	}
	return out
}

func expectWrites(t *testing.T, shell guardrule.Shell, src string, want ...string) {
	t.Helper()
	got := writes(t, shell, src)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("writes of %q\n got: %q\nwant: %q", src, got, want)
	}
}

// TestWriteRedirections: an output redirection writes its target, an
// input redirection does not (spec 0058, Desired Behavior 2).
func TestWriteRedirections(t *testing.T) {
	bash := []struct {
		src  string
		want []string
	}{
		{"echo x > f", []string{"f"}},
		{"echo x >> f", []string{"f"}},
		{"echo x >| f", []string{"f"}},
		{"echo x &> f", []string{"f"}},
		{"echo x &>> f", []string{"f"}},
		{"echo x 2> f", []string{"f"}},
		{"echo x 2>> f", []string{"f"}},
		{"echo x >& f", []string{"f"}},
		{"echo x 2>&1", nil},
		{"echo x >&-", nil},
		{"jq . < f", nil},
		{"cat <<EOF > f\nx\nEOF", []string{"f"}},
		{"cat <<< x", nil},
		{"{ echo x; } > f", []string{"f"}},
		{"> f", []string{"f"}},
		{`echo x > "$f"`, []string{"<$>"}},
		{"sudo echo x > f", []string{"f"}},
		{"bash -c 'echo x > f'", []string{"f"}},
	}
	for _, c := range bash {
		expectWrites(t, guardrule.ShellBash, c.src, c.want...)
	}
	ps := []struct {
		src  string
		want []string
	}{
		{"Get-Date > f", []string{"f"}},
		{"Get-Date >> f", []string{"f"}},
		{"Get-Date 2> f", []string{"f"}},
		{"Get-Date *> f", []string{"f"}},
		{"Get-Date 2>&1", nil},
		{"'x' > f", []string{"f"}},
	}
	for _, c := range ps {
		expectWrites(t, guardrule.ShellPowerShell, c.src, c.want...)
	}
}

// TestWritingPrograms: the operands that tee, sed -i, truncate, cp, mv,
// install, ln, rm, unlink and dd write. A destination that may be a
// directory also writes the source's name inside it.
func TestWritingPrograms(t *testing.T) {
	cases := []struct {
		src  string
		want []string
	}{
		{"tee f", []string{"f"}},
		{"tee -a f g", []string{"f", "g"}},
		{"echo x | tee --append -- f", []string{"f"}},
		{"sed -i s/a/b/ f", []string{"f"}},
		{"sed -i.bak s/a/b/ f g", []string{"f", "g"}},
		{"sed -ni s/a/b/p f", []string{"f"}},
		{"sed --in-place=.bak -e s/a/b/ f", []string{"f"}},
		{"sed -E -i -e s/a/b/ -f script.sed f", []string{"f"}},
		{"sed -i '' s/a/b/ f", []string{"s/a/b/", "f"}},
		{"sed s/a/b/ f", nil},
		{"sed -n p f", nil},
		{"truncate -s 0 f", []string{"f"}},
		{"truncate --size=0 f", []string{"f"}},
		{"truncate -r ref f", []string{"f"}},
		{"cp a b", []string{"b", "b/a"}},
		{"cp -r dir/a.json x/b", []string{"x/b", "x/b/a.json"}},
		{"cp -S .bak a b", []string{"b", "b/a"}},
		{"cp a b c", []string{"c", "c/a", "c/b"}},
		{"cp -t dir a b", []string{"dir", "dir/a", "dir/b"}},
		{"cp --target-directory=dir a", []string{"dir", "dir/a"}},
		{"cp -T a b", []string{"b"}},
		{"cp a", nil},
		{"mv a b", []string{"b", "b/a", "a"}},
		{"mv -t dir a b", []string{"dir", "dir/a", "dir/b", "a", "b"}},
		{"install -m 644 a b", []string{"b", "b/a"}},
		{"install -d x y", []string{"x", "y"}},
		{"ln -sf /x/a b", []string{"b", "b/a"}},
		{"ln -s /x/a", []string{"a"}},
		{"rm f", []string{"f"}},
		{"rm -rf a b", []string{"a", "b"}},
		{"rm -- -f", []string{"-f"}},
		{"unlink f", []string{"f"}},
		{"dd if=a of=b bs=1", []string{"b"}},
		{"dd if=a", nil},
		{"cat f", nil},
		{"rm $f", []string{"<?>"}},
		{"sudo rm f", []string{"f"}},
		{"xargs rm", []string{"<?>"}},
		{"xargs -I{} cp {} f", []string{"f", "f/<$>"}},
		{"find . -name x -exec rm {} \\;", []string{"{}"}},
		{"bash -c 'sed -i s/a/b/ f'", []string{"f"}},
		{"/usr/bin/tee f", []string{"f"}},
	}
	for _, c := range cases {
		expectWrites(t, guardrule.ShellBash, c.src, c.want...)
	}
}

// TestWritingCmdlets: the items that PowerShell's writing cmdlets name
// by -Path, -LiteralPath, -FilePath, -Destination or position. A name
// that is also a program (rm, cp, tee) is read both ways.
func TestWritingCmdlets(t *testing.T) {
	cases := []struct {
		src  string
		want []string
	}{
		{"Set-Content f x", []string{"f"}},
		{"Set-Content -Value x -Path f", []string{"f"}},
		{"Set-Content -Path:f -Value x", []string{"f"}},
		{"'x' | Set-Content -LiteralPath f -NoNewline", []string{"f"}},
		{"sc -Encoding utf8 f x", []string{"f"}},
		{"Add-Content f x", []string{"f"}},
		{"Get-Date | Out-File f", []string{"f"}},
		{"Get-Date | Out-File -FilePath f -Append", []string{"f"}},
		{"Get-Date | Out-File -Encoding utf8 -Path f", []string{"f"}},
		{"Clear-Content f", []string{"f"}},
		{"Remove-Item f", []string{"f"}},
		{"Remove-Item a, b -Force", []string{"a", "b"}},
		{"del f", []string{"f"}},
		{"Rename-Item d/x settings.json", []string{"d/x", "d/settings.json"}},
		{"Rename-Item -Path x -NewName y", []string{"x", "./y"}},
		{"New-Item -ItemType File f", []string{"f"}},
		{"New-Item -Path d -Name f -Value x", []string{"d", "d/f"}},
		{"Get-Date | Tee-Object f", []string{"f"}},
		{"Get-Date | Tee-Object -FilePath f", []string{"f"}},
		{"Copy-Item a -Destination d", []string{"d", "d/a"}},
		{"Copy-Item a d", []string{"d", "d/a"}},
		{"Move-Item -Path a -Destination d", []string{"d", "d/a", "a"}},
		{"rm f", []string{"f"}},
		{"cp a d", []string{"d", "d/a"}},
		{"Get-Content f", nil},
		{"Set-Content $f x", []string{"<?>"}},
		{"bash -c 'rm f'", []string{"f"}},
		{"pwsh -c 'Set-Content f x'", []string{"f"}},
	}
	for _, c := range cases {
		expectWrites(t, guardrule.ShellPowerShell, c.src, c.want...)
	}
}
