#!/bin/bash
# PreToolUse guard for Bash/PowerShell: deny obviously destructive command
# patterns. This is a guardrail, not a complete enforcement boundary — see
# CLAUDE.md and docs/decisions for what it does and does not cover.
#
# No dependency on jq/python/node: pattern-matches the raw stdin JSON blob,
# which is sufficient for substring detection and keeps this hook portable.

input="$(cat)"

deny() {
  printf '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"%s"}}' "$1"
  exit 0
}

if echo "$input" | grep -qE 'rm[[:space:]]+(-[a-zA-Z]*r[a-zA-Z]*f|-[a-zA-Z]*f[a-zA-Z]*r)([[:space:]]|"|$)'; then
  deny "Blocked: rm -rf (or equivalent) is a destructive operation. Ask the user to run it themselves if intended."
fi

if echo "$input" | grep -qE 'Remove-Item[^"\\]*-Recurse[^"\\]*-Force|Remove-Item[^"\\]*-Force[^"\\]*-Recurse'; then
  deny "Blocked: Remove-Item -Recurse -Force is a destructive operation. Ask the user to run it themselves if intended."
fi

if echo "$input" | grep -qE 'git[[:space:]]+reset[[:space:]]+--hard'; then
  deny "Blocked: git reset --hard discards uncommitted work. Ask the user to run it themselves if intended."
fi

if echo "$input" | grep -qE 'git[[:space:]]+clean[[:space:]]+-[a-zA-Z]*f[a-zA-Z]*d|git[[:space:]]+clean[[:space:]]+-[a-zA-Z]*d[a-zA-Z]*f'; then
  deny "Blocked: git clean -fd (or equivalent) permanently deletes untracked files. Ask the user to run it themselves if intended."
fi

if echo "$input" | grep -qE 'git[[:space:]]+push[^"\\]*(--force([[:space:]]|"|$)|[[:space:]]-f([[:space:]]|"|$))'; then
  deny "Blocked: git push --force can overwrite remote history. Ask the user to run it themselves if intended."
fi

if echo "$input" | grep -qE 'git[[:space:]]+branch[[:space:]]+-D'; then
  deny "Blocked: git branch -D force-deletes a branch. Ask the user to run it themselves if intended."
fi

exit 0
