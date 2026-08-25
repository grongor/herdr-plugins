#!/usr/bin/env bash
# PreToolUse/Bash guard for the user's live Herdr session.
#
# 1. No pattern kills: `pkill -f "herdr server"` once killed the live server while a test instance was being cleaned up.
#    Stop a test instance by its PID or with `HERDR_SOCKET_PATH=<iso.sock> herdr server stop`.
# 2. Mutating `herdr` commands must set HERDR_SOCKET_PATH= on the command itself, or they land on the default socket.
#
# Read-only commands (status, list, api schema, --help) stay allowed.

set -uo pipefail

command_text=$(jq -r '.tool_input.command // empty' 2>/dev/null)
[ -z "$command_text" ] && exit 0

deny() {
  jq -nc --arg reason "$1" '{
    hookSpecificOutput: {
      hookEventName: "PreToolUse",
      permissionDecision: "deny",
      permissionDecisionReason: $reason
    }
  }'
  exit 0
}

# Rule 1 — pattern kills, whatever they claim to target.
if grep -Eq '(^|[;&|(]|[[:space:]])(pkill|killall)([[:space:]]|$)' <<<"$command_text" ||
  grep -Eq 'pgrep[^|]*\|[[:space:]]*(xargs[[:space:]]+)?kill' <<<"$command_text" ||
  grep -Eq 'kill[[:space:]]+[^|;&]*[`$]\(?[[:space:]]*pgrep' <<<"$command_text"; then
  deny 'Blocked: pattern kill (pkill/killall/pgrep|kill). A pattern that matches a test process matches the user'"'"'s live one too — this is how the live Herdr server was killed once. Kill by the PID you recorded when you started the process, or stop an isolated Herdr with HERDR_SOCKET_PATH=<iso.sock> herdr server stop.'
fi

# Rule 2 — mutating herdr commands must name an isolated socket.
if grep -Eq '(^|[;&|(]|[[:space:]])herdr([[:space:]]|$)' <<<"$command_text" &&
  ! grep -Eq '(--help|[[:space:]]-h)([[:space:]]|$)' <<<"$command_text" &&
  ! grep -q 'HERDR_SOCKET_PATH=' <<<"$command_text"; then
  if grep -Eq 'herdr[^;&|]*[[:space:]](create|open|remove|close|delete|stop|start|attach|prompt|send-keys|invoke|link|unlink|install|uninstall|enable|disable|split|run|input|kill|move|resize|focus|reload-config|update|new|show|set)([[:space:]]|$)' <<<"$command_text"; then
    deny 'Blocked: this herdr command changes Herdr state and does not name an isolated instance. It would hit the user'"'"'s live session (see the never-touch-live-herdr memory). Run it against an isolated instance by setting HERDR_SOCKET_PATH=<iso.sock> on the command, or ask the user to run it themselves. Read-only commands (status, list, api schema, --help) are not blocked.'
  fi
fi

exit 0
