package textregion

import (
	"bytes"
	"strings"
	"testing"
)

const (
	begin = "# vibeconform:begin le\n"
	end   = "# vibeconform:end le\n"
)

func TestFindWellFormed(t *testing.T) {
	data := "*.png binary\n" + begin + "* text=auto eol=lf\n" + end + "*.bat eol=crlf\n"
	d := Parse([]byte(data), Hash)
	span, ok, err := d.Find("le")
	if err != nil || !ok {
		t.Fatalf("Find = %v, %v", ok, err)
	}
	if got := string(d.Inner(span)); got != "* text=auto eol=lf\n" {
		t.Errorf("Inner = %q", got)
	}
	if got := data[span.Start:span.End]; got != begin+"* text=auto eol=lf\n"+end {
		t.Errorf("block = %q", got)
	}
	if _, ok, err := d.Find("other"); ok || err != nil {
		t.Errorf("absent id: ok %v, err %v", ok, err)
	}
}

func TestFindToleratesTrailingSpaceAndCR(t *testing.T) {
	data := "# vibeconform:begin le \t\r\nx\r\n# vibeconform:end le\r\n"
	d := Parse([]byte(data), Hash)
	span, ok, err := d.Find("le")
	if err != nil || !ok {
		t.Fatalf("Find = %v, %v", ok, err)
	}
	// The content is the bytes on disk: a CRLF line stays CRLF, so it
	// hashes differently from an LF target, which is drift by design.
	if got := string(d.Inner(span)); got != "x\r\n" {
		t.Errorf("Inner = %q", got)
	}
}

func TestFindHTML(t *testing.T) {
	data := "# Title\n\n<!-- vibeconform:begin workflow -->\nRead docs/.\n<!-- vibeconform:end workflow -->\n"
	d := Parse([]byte(data), HTML)
	span, ok, err := d.Find("workflow")
	if err != nil || !ok {
		t.Fatalf("Find = %v, %v", ok, err)
	}
	if got := string(d.Inner(span)); got != "Read docs/.\n" {
		t.Errorf("Inner = %q", got)
	}
	// Markers of the other syntax are ordinary text.
	if _, ok, _ := Parse([]byte(data), Hash).Find("workflow"); ok {
		t.Error("HTML markers matched as Hash markers")
	}
}

func TestFindMalformed(t *testing.T) {
	for name, tc := range map[string]struct{ data, want string }{
		"begin only":      {begin + "a\n", "begin marker for le at line 1 has no end marker"},
		"end only":        {"a\n" + end, "end marker for le at line 2 has no begin marker before it"},
		"end before":      {end + begin, "end marker for le at line 1 has no begin marker before it"},
		"duplicate":       {begin + end + begin + end, "a second begin marker for le at line 3"},
		"overlap":         {begin + "# vibeconform:begin x\n" + end + "# vibeconform:end x\n", "section le (line 1) overlaps section x (line 2)"},
		"nested":          {"# vibeconform:begin x\n" + begin + end + "# vibeconform:end x\n", "section le (line 2) overlaps section x (line 1)"},
		"closed by other": {begin + "# vibeconform:end x\n", "section le (line 1) is closed by the end marker of x (line 2)"},
	} {
		t.Run(name, func(t *testing.T) {
			_, ok, err := Parse([]byte(tc.data), Hash).Find("le")
			if ok || err == nil || err.Error() != tc.want {
				t.Fatalf("Find = %v, %v; want error %q", ok, err, tc.want)
			}
		})
	}
}

// TestMalformedSectionLeavesOthers: one broken section does not hide a
// well-formed one elsewhere in the file.
func TestMalformedSectionLeavesOthers(t *testing.T) {
	data := "# vibeconform:begin x\nx\n# vibeconform:end x\n" + begin
	d := Parse([]byte(data), Hash)
	if _, ok, err := d.Find("x"); !ok || err != nil {
		t.Errorf("x: ok %v, err %v", ok, err)
	}
	if _, _, err := d.Find("le"); err == nil {
		t.Error("le: want an error")
	}
}

func TestNearMissesAreText(t *testing.T) {
	for _, line := range []string{
		"# vibeconform:begin\n",
		"# vibeconform:start le\n",
		" # vibeconform:begin le\n",
		"#vibeconform:begin le\n",
		"# vibeconform:begin le extra\n",
	} {
		if _, ok, err := Parse([]byte(line), Hash).Find("le"); ok || err != nil {
			t.Errorf("%q: ok %v, err %v; want plain text", line, ok, err)
		}
	}
}

func TestInsert(t *testing.T) {
	block := begin + "r\n" + end
	for name, tc := range map[string]struct {
		data string
		at   Placement
		want string
	}{
		"empty top":          {"", Top, block},
		"empty bottom":       {"", Bottom, block},
		"top":                {"*.png binary\n", Top, block + "\n*.png binary\n"},
		"top keeps CRLF":     {"*.png binary\r\n", Top, block + "\n*.png binary\r\n"},
		"bottom":             {"# Title\n", Bottom, "# Title\n\n" + block},
		"bottom, no newline": {"# Title", Bottom, "# Title\n\n" + block},
	} {
		t.Run(name, func(t *testing.T) {
			if got := string(Insert([]byte(tc.data), Hash, "le", []byte("r\n"), tc.at)); got != tc.want {
				t.Errorf("Insert = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestReplaceKeepsEverythingElse(t *testing.T) {
	data := "a\r\n" + begin + "old\n" + end + "b\r\n"
	d := Parse([]byte(data), Hash)
	span, _, _ := d.Find("le")
	got := string(Replace([]byte(data), span, []byte("new\nlines\n")))
	if want := "a\r\n" + begin + "new\nlines\n" + end + "b\r\n"; got != want {
		t.Errorf("Replace = %q, want %q", got, want)
	}
}

// TestRemoveUndoesInsert: inserting then removing gives back the bytes the
// file had, for every placement and line ending.
func TestRemoveUndoesInsert(t *testing.T) {
	for _, data := range []string{"", "*.png binary\n", "a\r\nb\r\n", "# Title\n\nText.\n"} {
		for _, at := range []Placement{Top, Bottom} {
			inserted := Insert([]byte(data), Hash, "le", []byte("r\n"), at)
			span, ok, err := Parse(inserted, Hash).Find("le")
			if !ok || err != nil {
				t.Fatalf("%q at %v: Find = %v, %v", data, at, ok, err)
			}
			if got := string(Remove(inserted, span, at)); got != data {
				t.Errorf("%q at %v: Remove = %q", data, at, got)
			}
		}
	}
}

// TestRemoveFirstBottomSection is the regression test for plan 0043: two
// Bottom sections inserted into an empty file, then the first removed, left
// a leading empty line, so the file never became empty again and was not
// deleted when the second section left too.
func TestRemoveFirstBottomSection(t *testing.T) {
	for _, nl := range []string{"\n", "\r\n"} {
		data := Insert(nil, HTML, "a", []byte("x"+nl), Bottom)
		data = bytes.ReplaceAll(data, []byte("\n"), []byte(nl))
		data = Insert(data, HTML, "b", []byte("y"+nl), Bottom)
		span, _, _ := Parse(data, HTML).Find("a")
		data = Remove(data, span, Bottom)
		if want := Begin(HTML, "b") + "\n" + "y" + nl + End(HTML, "b") + "\n"; string(data) != want {
			t.Errorf("%q: after removing a: %q, want %q", nl, data, want)
		}
		span, _, _ = Parse(data, HTML).Find("b")
		if got := Remove(data, span, Bottom); len(got) != 0 {
			t.Errorf("%q: after removing both: %q, want empty", nl, got)
		}
	}
}

// TestRemoveKeepsUserLines: a user line written where the empty line was is
// not removed with the section.
func TestRemoveKeepsUserLines(t *testing.T) {
	data := begin + "r\n" + end + "*.png binary\n"
	span, _, _ := Parse([]byte(data), Hash).Find("le")
	if got := string(Remove([]byte(data), span, Top)); got != "*.png binary\n" {
		t.Errorf("Remove = %q", got)
	}
}

func TestBlockHasNoStrayMarkers(t *testing.T) {
	b := string(Block(HTML, "workflow", []byte("x\n")))
	if strings.Count(b, "vibeconform:") != 2 || !strings.HasSuffix(b, "<!-- vibeconform:end workflow -->\n") {
		t.Errorf("Block = %q", b)
	}
}
