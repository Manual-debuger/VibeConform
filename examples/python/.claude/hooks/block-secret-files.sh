#!/bin/bash
# PreToolUse guard for Write/Edit: deny direct edits to files that
# conventionally hold secrets. Not a scanner for secret *content* — just a
# filename-pattern guardrail. See CLAUDE.md.

input="$(cat)"

deny() {
  printf '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"%s"}}' "$1"
  exit 0
}

if echo "$input" | grep -qE '"file_path"[[:space:]]*:[[:space:]]*"[^"]*(\.env(\.[a-zA-Z]+)?|\.pem|\.key|id_rsa|id_ed25519|credentials\.json|\.npmrc|\.netrc)"'; then
  deny "Blocked: this looks like a secret/credential file. Ask the user to edit it themselves."
fi

exit 0
