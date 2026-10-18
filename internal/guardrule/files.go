package guardrule

import (
	"errors"
	"strings"
	"time"
)

// The files the contract audit, vibe-guard and vibe unlock must agree on
// (spec 0051, "Guard policy and exceptions" and "The unlock marker").

// PolicyFile is the guard's policy, relative to the repository root.
const PolicyFile = ".claude/hooks/policy.json"

// LocalPolicyFile is the runtime guards' personal adjustments (spec
// 0050). vibe-guard does not read it, and the audit never counts it.
const LocalPolicyFile = ".claude/hooks/policy.local.json"

// UnlockMarkerFile is the unlock marker's name in the common Git
// directory.
const UnlockMarkerFile = "vibe-unlock"

// FormatUnlockMarker is the content of an unlock marker that expires at
// expiry: one line, the expiry in RFC 3339 UTC, to the second.
func FormatUnlockMarker(expiry time.Time) []byte {
	return []byte(expiry.UTC().Format(time.RFC3339) + "\n")
}

// ParseUnlockMarker reads an unlock marker's expiry: the first line,
// one RFC 3339 time.
func ParseUnlockMarker(data []byte) (time.Time, error) {
	line, _, _ := strings.Cut(string(data), "\n")
	expiry, err := time.Parse(time.RFC3339, strings.TrimSpace(line))
	if err != nil {
		return time.Time{}, errors.New("the unlock marker does not hold one RFC 3339 time")
	}
	return expiry, nil
}
