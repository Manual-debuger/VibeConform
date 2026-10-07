// Package textregion finds, inserts, replaces, and removes marker-delimited
// sections of a text file that is otherwise someone else's
// (docs/decisions/0014-managed-sections.md).
//
// A section is the lines strictly between a begin marker and an end
// marker, each on a line of its own. Markers only say where a section is;
// whether a caller owns it is the caller's business. Every byte outside the
// section a call touches is copied verbatim, so a file's CRLF lines stay
// CRLF and its user content is never reformatted.
package textregion

import (
	"bytes"
	"fmt"
	"strings"
)

// Syntax is the comment syntax a file's markers are written in.
type Syntax int

const (
	// Hash writes "# vibeconform:begin <id>": gitattributes, YAML, TOML,
	// shell.
	Hash Syntax = iota
	// HTML writes "<!-- vibeconform:begin <id> -->": Markdown.
	HTML
)

// Placement is where a new section goes in an existing file.
type Placement int

const (
	// Top inserts the section, then one empty line, before the file's bytes.
	Top Placement = iota
	// Bottom appends one empty line, then the section, after them.
	Bottom
)

const markerWord = "vibeconform:"

// Begin returns the begin marker line for id, without a line ending.
func Begin(s Syntax, id string) string { return marker(s, "begin", id) }

// End returns the end marker line for id, without a line ending.
func End(s Syntax, id string) string { return marker(s, "end", id) }

func marker(s Syntax, kind, id string) string {
	if s == HTML {
		return "<!-- " + markerWord + kind + " " + id + " -->"
	}
	return "# " + markerWord + kind + " " + id
}

// Span locates one well-formed section in a file, as byte offsets.
type Span struct {
	// Start is where the begin marker line starts.
	Start int
	// InnerStart is just after the begin marker line's newline.
	InnerStart int
	// InnerEnd is where the end marker line starts.
	InnerEnd int
	// End is just after the end marker line's newline, or the end of the
	// file when that line has none.
	End int
}

// Doc is a parsed file: every section it holds, and why each malformed one
// is malformed.
type Doc struct {
	data   []byte
	syntax Syntax
	spans  map[string]Span
	errs   map[string]string
}

// Parse finds every section in data written in syntax. It never fails: a
// malformed section is reported per ID by Find, so one broken section does
// not hide the others.
func Parse(data []byte, syntax Syntax) *Doc {
	d := &Doc{data: data, syntax: syntax, spans: map[string]Span{}, errs: map[string]string{}}

	type open struct {
		id         string
		line       int
		start, end int
	}
	var cur *open
	seen := map[string]bool{}
	fail := func(id, why string) {
		if _, ok := d.errs[id]; !ok {
			d.errs[id] = why
		}
		delete(d.spans, id)
	}

	line, off := 0, 0
	for off < len(data) {
		line++
		next := bytes.IndexByte(data[off:], '\n')
		lineEnd := len(data)
		if next >= 0 {
			lineEnd = off + next + 1
		}
		kind, id, ok := d.match(data[off:lineEnd])
		switch {
		case !ok:
		case kind == "begin" && cur != nil:
			fail(cur.id, fmt.Sprintf("section %s (line %d) overlaps section %s (line %d)", cur.id, cur.line, id, line))
			fail(id, fmt.Sprintf("section %s (line %d) overlaps section %s (line %d)", id, line, cur.id, cur.line))
			cur = nil
		case kind == "begin" && seen[id]:
			fail(id, fmt.Sprintf("a second begin marker for %s at line %d", id, line))
		case kind == "begin":
			seen[id] = true
			cur = &open{id: id, line: line, start: off, end: lineEnd}
		case cur == nil:
			fail(id, fmt.Sprintf("end marker for %s at line %d has no begin marker before it", id, line))
		case cur.id != id:
			fail(cur.id, fmt.Sprintf("section %s (line %d) is closed by the end marker of %s (line %d)", cur.id, cur.line, id, line))
			fail(id, fmt.Sprintf("end marker for %s at line %d has no begin marker before it", id, line))
			cur = nil
		default:
			if _, broken := d.errs[id]; !broken {
				d.spans[id] = Span{Start: cur.start, InnerStart: cur.end, InnerEnd: off, End: lineEnd}
			}
			cur = nil
		}
		off = lineEnd
	}
	if cur != nil {
		fail(cur.id, fmt.Sprintf("begin marker for %s at line %d has no end marker", cur.id, cur.line))
	}
	return d
}

// match reports whether line is a marker line, and which.
func (d *Doc) match(line []byte) (kind, id string, ok bool) {
	text := strings.TrimRight(string(line), " \t\r\n")
	prefix, suffix := "# "+markerWord, ""
	if d.syntax == HTML {
		prefix, suffix = "<!-- "+markerWord, " -->"
	}
	rest, found := strings.CutPrefix(text, prefix)
	if !found {
		return "", "", false
	}
	if rest, found = strings.CutSuffix(rest, suffix); !found {
		return "", "", false
	}
	kind, id, found = strings.Cut(rest, " ")
	if !found || (kind != "begin" && kind != "end") || id == "" || strings.ContainsAny(id, " \t") {
		return "", "", false
	}
	return kind, id, true
}

// Find returns id's section. ok is false when the file has no marker for
// id at all; err is set when its markers are malformed.
func (d *Doc) Find(id string) (span Span, ok bool, err error) {
	if why, broken := d.errs[id]; broken {
		return Span{}, false, fmt.Errorf("%s", why)
	}
	span, ok = d.spans[id]
	return span, ok, nil
}

// Inner returns the content of span: the bytes between its markers.
func (d *Doc) Inner(span Span) []byte {
	return d.data[span.InnerStart:span.InnerEnd]
}

// Block returns id's whole section as written: begin marker, content, end
// marker, each line ending in "\n". content must be empty or end in "\n".
func Block(s Syntax, id string, content []byte) []byte {
	var b bytes.Buffer
	b.WriteString(Begin(s, id))
	b.WriteByte('\n')
	b.Write(content)
	b.WriteString(End(s, id))
	b.WriteByte('\n')
	return b.Bytes()
}

// Insert returns data with id's section added at placement. An empty file
// becomes the section alone.
func Insert(data []byte, s Syntax, id string, content []byte, at Placement) []byte {
	block := Block(s, id, content)
	if len(data) == 0 {
		return block
	}
	var b bytes.Buffer
	if at == Top {
		b.Write(block)
		b.WriteString("\n")
		b.Write(data)
		return b.Bytes()
	}
	b.Write(data)
	if data[len(data)-1] != '\n' {
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.Write(block)
	return b.Bytes()
}

// Replace returns data with span's content replaced; the marker lines and
// everything outside them are kept.
func Replace(data []byte, span Span, content []byte) []byte {
	out := make([]byte, 0, len(data)-(span.InnerEnd-span.InnerStart)+len(content))
	out = append(out, data[:span.InnerStart]...)
	out = append(out, content...)
	return append(out, data[span.InnerEnd:]...)
}

// Remove returns data without span's section, and without the one empty
// line Insert put next to it at placement, if that line is still there.
// A Bottom section that starts the file was inserted into an empty file,
// with no empty line before it; the one after it was put there by the
// next Bottom section, and goes too, so that section starts the file.
func Remove(data []byte, span Span, at Placement) []byte {
	before, after := data[:span.Start], data[span.End:]
	switch {
	case at == Bottom && len(before) == 0 && bytes.HasPrefix(after, []byte("\r\n")):
		after = after[2:]
	case at == Bottom && len(before) == 0 && bytes.HasPrefix(after, []byte("\n")):
		after = after[1:]
	case at == Top && bytes.HasPrefix(after, []byte("\r\n")):
		after = after[2:]
	case at == Top && bytes.HasPrefix(after, []byte("\n")):
		after = after[1:]
	case at == Bottom && bytes.HasSuffix(before, []byte("\n\r\n")):
		before = before[:len(before)-2]
	case at == Bottom && bytes.HasSuffix(before, []byte("\n\n")):
		before = before[:len(before)-1]
	}
	out := make([]byte, 0, len(before)+len(after))
	out = append(out, before...)
	return append(out, after...)
}
