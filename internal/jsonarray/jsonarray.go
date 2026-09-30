// Package jsonarray edits owned elements of one array inside a JSONC
// document — JSON with comments and trailing commas, as VS Code and Zed
// write it — while every byte outside those elements stays exactly as it
// was. It is the mechanism behind structured-patch resources; see
// docs/decisions/0013-optional-integrations.md.
//
// An element is identified by its "label" member (an object) or by its
// value (a string). Elements with neither are not addressable and are
// never touched.
package jsonarray

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/tailscale/hujson"
)

// Doc is a parsed JSONC document and the array it edits.
type Doc struct {
	root hujson.Value
	// key is the array's member name in a root object; "" when the root is
	// the array itself.
	key string
	// newline is "\r\n" for a document that uses it, so inserted lines
	// match the rest of the file.
	newline string
}

// Parse parses data and locates the array named key: a member of the
// root object, or the root itself when key is empty. A root object with no
// such member gets an empty one appended. Anything else — invalid JSONC, a
// root of the wrong kind, a member that is not an array — is an error.
func Parse(data []byte, key string) (*Doc, error) {
	root, err := hujson.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("not valid JSON with comments: %w", err)
	}
	d := &Doc{root: root, key: key, newline: "\n"}
	if bytes.Contains(data, []byte("\r\n")) {
		d.newline = "\r\n"
	}

	if key == "" {
		if _, ok := root.Value.(*hujson.Array); !ok {
			return nil, errors.New("the document is not an array")
		}
		return d, nil
	}
	obj, ok := root.Value.(*hujson.Object)
	if !ok {
		return nil, errors.New("the document is not an object")
	}
	for i := range obj.Members {
		if name, ok := obj.Members[i].Name.Value.(hujson.Literal); ok && name.String() == key {
			if _, ok := obj.Members[i].Value.Value.(*hujson.Array); !ok {
				return nil, fmt.Errorf("%q is not an array", key)
			}
			return d, nil
		}
	}
	d.addMember(obj)
	return d, nil
}

// addMember appends "key": [] to obj, indented like its last member.
func (d *Doc) addMember(obj *hujson.Object) {
	before := hujson.Extra(d.newline + "  ")
	trailing := false
	if n := len(obj.Members); n > 0 {
		before = hujson.Extra(d.newline + indentOf(obj.Members[n-1].Name.BeforeExtra))
		trailing = obj.Members[n-1].Value.AfterExtra != nil
		if trailing {
			// The old last member keeps its separator; the trailing comma
			// moves to the new last member below.
			obj.Members[n-1].Value.AfterExtra = nil
		}
	} else if !hasComment(obj.AfterExtra) {
		obj.AfterExtra = hujson.Extra(d.newline)
	}
	m := hujson.ObjectMember{
		Name:  hujson.Value{BeforeExtra: before, Value: hujson.String(d.key)},
		Value: hujson.Value{BeforeExtra: hujson.Extra(" "), Value: &hujson.Array{}},
	}
	if trailing {
		m.Value.AfterExtra = hujson.Extra{}
	}
	obj.Members = append(obj.Members, m)
}

func (d *Doc) array() *hujson.Array {
	if d.key == "" {
		return d.root.Value.(*hujson.Array)
	}
	for _, m := range d.root.Value.(*hujson.Object).Members {
		if name, ok := m.Name.Value.(hujson.Literal); ok && name.String() == d.key {
			return m.Value.Value.(*hujson.Array)
		}
	}
	panic("jsonarray: array vanished") // Parse guarantees it exists
}

// Find returns the index of the element identified by id, or -1. Two
// elements with the same identity are an error: which one is owned would
// be a guess.
func (d *Doc) Find(id string) (int, error) {
	found := -1
	for i, e := range d.array().Elements {
		if got, ok := Identity(e); ok && got == id {
			if found >= 0 {
				return -1, fmt.Errorf("two elements are identified as %q", id)
			}
			found = i
		}
	}
	return found, nil
}

// Hash returns the canonical hash of element i; see HashValue.
func (d *Doc) Hash(i int) (string, error) {
	return HashValue(d.array().Elements[i].Pack())
}

// Append adds value, JSON text, as the array's last element, on its own
// line at the indentation the array already uses.
func (d *Doc) Append(value []byte) error {
	v, err := parseElement(value)
	if err != nil {
		return err
	}
	arr := d.array()
	if n := len(arr.Elements); n > 0 {
		last := &arr.Elements[n-1]
		v.BeforeExtra = hujson.Extra(d.newline + indentOf(last.BeforeExtra))
		if !bytes.Contains(last.BeforeExtra, []byte("\n")) {
			v.BeforeExtra = hujson.Extra(" ") // an inline array stays inline
		}
		if last.AfterExtra != nil {
			// Keep the trailing comma after the new last element.
			last.AfterExtra = nil
			v.AfterExtra = hujson.Extra{}
		}
	} else {
		d.root.UpdateOffsets()
		outer := d.lineIndent(arr)
		v.BeforeExtra = hujson.Extra(d.newline + outer + "  ")
		if !hasComment(arr.AfterExtra) {
			arr.AfterExtra = hujson.Extra(d.newline + outer)
		}
	}
	arr.Elements = append(arr.Elements, v)
	return nil
}

// Replace swaps element i's value for value, keeping the whitespace and
// comments around it.
func (d *Doc) Replace(i int, value []byte) error {
	v, err := parseElement(value)
	if err != nil {
		return err
	}
	e := &d.array().Elements[i]
	e.Value = v.Value
	return nil
}

// Remove deletes element i. A comment written above it is not the
// element's to take: it moves to whatever follows.
func (d *Doc) Remove(i int) {
	arr := d.array()
	removed := arr.Elements[i]
	arr.Elements = append(arr.Elements[:i], arr.Elements[i+1:]...)

	carry := hujson.Extra(nil)
	if hasComment(removed.BeforeExtra) {
		b := removed.BeforeExtra
		if nl := bytes.LastIndexByte(b, '\n'); nl >= 0 {
			b = b[:nl]
			b = bytes.TrimSuffix(b, []byte("\r"))
		}
		carry = b
	}

	switch {
	case i < len(arr.Elements):
		next := &arr.Elements[i]
		next.BeforeExtra = append(append(hujson.Extra{}, carry...), next.BeforeExtra...)
	case len(arr.Elements) > 0:
		last := &arr.Elements[len(arr.Elements)-1]
		if removed.AfterExtra != nil && last.AfterExtra == nil {
			last.AfterExtra = hujson.Extra{} // it was the trailing comma's
		}
		arr.AfterExtra = append(append(hujson.Extra{}, carry...), arr.AfterExtra...)
	default:
		// Empty now: collapse to [] unless a comment has to stay.
		arr.AfterExtra = carry
		if hasComment(arr.AfterExtra) {
			d.root.UpdateOffsets()
			arr.AfterExtra = append(arr.AfterExtra, hujson.Extra(d.newline+d.lineIndent(arr))...)
		}
	}
}

// Len is the number of elements in the array.
func (d *Doc) Len() int {
	return len(d.array().Elements)
}

// Bytes packs the document.
func (d *Doc) Bytes() []byte {
	return d.root.Pack()
}

// lineIndent is the leading whitespace of the line the array opens on.
func (d *Doc) lineIndent(arr *hujson.Array) string {
	packed := d.root.Pack()
	start := -1
	for v := range d.root.All() {
		if a, ok := v.Value.(*hujson.Array); ok && a == arr {
			start = v.StartOffset
			break
		}
	}
	if start < 0 {
		return ""
	}
	lineStart := bytes.LastIndexByte(packed[:start], '\n') + 1
	i := lineStart
	for i < start && (packed[i] == ' ' || packed[i] == '\t') {
		i++
	}
	return string(packed[lineStart:i])
}

// Identity returns an element's identity: an object's "label", or a
// string's value.
func Identity(v hujson.Value) (string, bool) {
	switch t := v.Value.(type) {
	case hujson.Literal:
		if t.Kind() == '"' {
			return t.String(), true
		}
	case *hujson.Object:
		for _, m := range t.Members {
			name, ok := m.Name.Value.(hujson.Literal)
			if !ok || name.String() != "label" {
				continue
			}
			if label, ok := m.Value.Value.(hujson.Literal); ok && label.Kind() == '"' {
				return label.String(), true
			}
		}
	}
	return "", false
}

// Canonical returns value's canonical JSON: comments, whitespace, and
// member order do not count, so reformatting an element is not editing
// it.
func Canonical(value []byte) ([]byte, error) {
	std, err := hujson.Standardize(append([]byte{}, value...))
	if err != nil {
		return nil, err
	}
	var v any
	if err := json.Unmarshal(std, &v); err != nil {
		return nil, err
	}
	return json.Marshal(v)
}

// HashValue is the hex SHA-256 of value's canonical JSON.
func HashValue(value []byte) (string, error) {
	c, err := Canonical(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(c)
	return hex.EncodeToString(sum[:]), nil
}

func parseElement(value []byte) (hujson.Value, error) {
	v, err := hujson.Parse(value)
	if err != nil {
		return hujson.Value{}, fmt.Errorf("element %s: %w", value, err)
	}
	v.BeforeExtra, v.AfterExtra = nil, nil
	return v, nil
}

// indentOf is the whitespace after the last newline in b.
func indentOf(b []byte) string {
	i := bytes.LastIndexByte(b, '\n')
	if i < 0 {
		return ""
	}
	rest := b[i+1:]
	n := 0
	for n < len(rest) && (rest[n] == ' ' || rest[n] == '\t') {
		n++
	}
	return string(rest[:n])
}

func hasComment(b []byte) bool {
	return bytes.Contains(b, []byte("//")) || bytes.Contains(b, []byte("/*"))
}
