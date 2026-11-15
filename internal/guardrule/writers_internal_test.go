package guardrule

import (
	"slices"
	"strings"
	"testing"
)

func literalWords(src string) []Word {
	var out []Word
	for _, f := range strings.Fields(src) {
		out = append(out, literalWord(f))
	}
	return out
}

func wordTexts(ws []Word) []string {
	var out []string
	for _, w := range ws {
		out = append(out, w.Text)
	}
	return out
}

// The writer scan fails closed (spec 0062, "The writer scanner keeps its
// decisions"): when a writer's options read more than one way, it writes
// what any reading writes; when they give too many readings to read, it
// writes every argument. No writer table reads an option both ways
// today, so a table that does is made here.
func TestWriterScanFailsClosed(t *testing.T) {
	both := optionTable{unknownLong: argUnknown, abbrev: true, minAbbrev: 2, permute: true}
	firstOperand := readWriter(both, func(r optionReading) []Word { return r.operands[:min(1, len(r.operands))] })

	// --zz a b reads as --zz with operands a b, and as --zz=a with b.
	if got, want := wordTexts(firstOperand(literalWords("--zz a b"))), []string{"a", "b"}; !slices.Equal(got, want) {
		t.Errorf("two readings: writes %q, want %q", got, want)
	}

	src := strings.Repeat("--zz ", maxReadings) + "f"
	if got, want := wordTexts(firstOperand(literalWords(src))), strings.Fields(src); !slices.Equal(got, want) {
		t.Errorf("too many readings: writes %q, want every argument %q", got, want)
	}
}
