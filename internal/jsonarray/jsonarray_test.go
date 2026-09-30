package jsonarray

import (
	"strings"
	"testing"
)

const vscodeUser = `// Project tasks.
{
    "version": "2.0.0",
    "tasks": [
        // Our release script.
        {
            "label": "release",
            "type": "shell",
            "command": "./release.sh", // keep in sync with CI
        },
    ],
}
`

func mustParse(t *testing.T, data, key string) *Doc {
	t.Helper()
	d, err := Parse([]byte(data), key)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// TestAppendPreservesEverythingElse: comments, trailing commas, and
// four-space indentation around the user's element survive byte for byte.
func TestAppendPreservesEverythingElse(t *testing.T) {
	d := mustParse(t, vscodeUser, "tasks")
	if err := d.Append([]byte(`{"label": "task verify", "command": "task"}`)); err != nil {
		t.Fatal(err)
	}
	want := `// Project tasks.
{
    "version": "2.0.0",
    "tasks": [
        // Our release script.
        {
            "label": "release",
            "type": "shell",
            "command": "./release.sh", // keep in sync with CI
        },
        {"label": "task verify", "command": "task"},
    ],
}
`
	if got := string(d.Bytes()); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}

	// And removing it again restores the original exactly.
	i, err := d.Find("task verify")
	if err != nil || i != 1 {
		t.Fatalf("Find = %d, %v", i, err)
	}
	d.Remove(i)
	if got := string(d.Bytes()); got != vscodeUser {
		t.Errorf("after remove\n%s\nwant the original\n%s", got, vscodeUser)
	}
}

func TestAppendNoTrailingComma(t *testing.T) {
	d := mustParse(t, "{\n  \"recommendations\": [\n    \"a.b\"\n  ]\n}\n", "recommendations")
	if err := d.Append([]byte(`"golang.go"`)); err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"recommendations\": [\n    \"a.b\",\n    \"golang.go\"\n  ]\n}\n"
	if got := string(d.Bytes()); got != want {
		t.Errorf("got\n%q\nwant\n%q", got, want)
	}
}

func TestAppendIntoEmptyArray(t *testing.T) {
	d := mustParse(t, "{\n  \"version\": \"2.0.0\",\n  \"tasks\": []\n}\n", "tasks")
	for _, e := range []string{`{"label": "a"}`, `{"label": "b"}`} {
		if err := d.Append([]byte(e)); err != nil {
			t.Fatal(err)
		}
	}
	want := "{\n  \"version\": \"2.0.0\",\n  \"tasks\": [\n    {\"label\": \"a\"},\n    {\"label\": \"b\"}\n  ]\n}\n"
	if got := string(d.Bytes()); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestTopLevelArray(t *testing.T) {
	d := mustParse(t, "[]\n", "")
	if err := d.Append([]byte(`{"label": "task test"}`)); err != nil {
		t.Fatal(err)
	}
	if got, want := string(d.Bytes()), "[\n  {\"label\": \"task test\"}\n]\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCRLFIsKept(t *testing.T) {
	in := "{\r\n  \"tasks\": []\r\n}\r\n"
	d := mustParse(t, in, "tasks")
	if err := d.Append([]byte(`{"label": "x"}`)); err != nil {
		t.Fatal(err)
	}
	got := string(d.Bytes())
	if strings.Count(got, "\n") != strings.Count(got, "\r\n") {
		t.Errorf("mixed line endings: %q", got)
	}
}

func TestMissingArrayIsAdded(t *testing.T) {
	d := mustParse(t, "{\n  \"version\": \"2.0.0\"\n}\n", "tasks")
	if err := d.Append([]byte(`{"label": "x"}`)); err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"version\": \"2.0.0\",\n  \"tasks\": [\n    {\"label\": \"x\"}\n  ]\n}\n"
	if got := string(d.Bytes()); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestReplaceKeepsSurroundings(t *testing.T) {
	d := mustParse(t, "[\n  // mine\n  {\"label\": \"x\", \"v\": 1}, // after\n  \"s\"\n]\n", "")
	i, _ := d.Find("x")
	if err := d.Replace(i, []byte(`{"label": "x", "v": 2}`)); err != nil {
		t.Fatal(err)
	}
	want := "[\n  // mine\n  {\"label\": \"x\", \"v\": 2}, // after\n  \"s\"\n]\n"
	if got := string(d.Bytes()); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestRemoveCarriesCommentAbove(t *testing.T) {
	d := mustParse(t, "[\n  // keep me\n  \"a\",\n  \"b\"\n]\n", "")
	i, _ := d.Find("a")
	d.Remove(i)
	if got, want := string(d.Bytes()), "[\n  // keep me\n  \"b\"\n]\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRemoveLastToEmpty(t *testing.T) {
	d := mustParse(t, "{\n  \"tasks\": [\n    {\"label\": \"a\"}\n  ]\n}\n", "tasks")
	i, _ := d.Find("a")
	d.Remove(i)
	if got, want := string(d.Bytes()), "{\n  \"tasks\": []\n}\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if d.Len() != 0 {
		t.Errorf("Len = %d", d.Len())
	}
}

func TestParseRejects(t *testing.T) {
	for name, tc := range map[string]struct{ in, key, want string }{
		"invalid":        {"{", "tasks", "not valid JSON"},
		"not an object":  {"[]", "tasks", "not an object"},
		"not an array":   {"{}", "", "not an array"},
		"member is not":  {`{"tasks": {}}`, "tasks", `"tasks" is not an array`},
		"duplicate find": {`["a", "a"]`, "", ""},
	} {
		t.Run(name, func(t *testing.T) {
			d, err := Parse([]byte(tc.in), tc.key)
			if tc.want == "" {
				if _, err := d.Find("a"); err == nil {
					t.Fatal("two elements with one identity: no error")
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %v, want %q", err, tc.want)
			}
		})
	}
}

func TestHashIgnoresFormatting(t *testing.T) {
	a, err := HashValue([]byte(`{"label": "x", "args": ["y"]}`))
	if err != nil {
		t.Fatal(err)
	}
	b, err := HashValue([]byte("{\n  // c\n  \"args\": [\"y\",],\n  \"label\": \"x\",\n}"))
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Error("reformatting an element changed its hash")
	}
	c, _ := HashValue([]byte(`{"label": "x", "args": ["z"]}`))
	if a == c {
		t.Error("a changed value kept its hash")
	}
}
