#!/usr/bin/env bash

set -u

ORIG_PATH="$PATH"

PATH="${HOME}/.local/bin:/usr/local/bin:/opt/homebrew/bin:${PATH}"
export PATH

VERSION="${VERSION:-unknown}"

printf 'ai-setup %s\n' "$VERSION"

required_failures=0
warnings=0

section() {
	printf '\n== %s ==\n' "$1"
}

ok() {
	printf '\033[32m[OK]\033[0m %s\n' "$1"
}

fail() {
	printf '\033[31m[FAIL]\033[0m %s\n' "$1"
	required_failures=$((required_failures + 1))
}

warn() {
	printf '\033[33m[WARN]\033[0m %s\n' "$1"
	warnings=$((warnings + 1))
}

note() {
	printf '       %s\n' "$1"
}

compact_output() {
	printf '%s' "$1" |
		tr '\n' ' ' |
		sed 's/[[:space:]][[:space:]]*/ /g; s/^ //; s/ $//' |
		cut -c1-220
}

extract_json() {
	printf '%s\n' "$1" | awk '
    BEGIN { start = 0 }
    /^[[:space:]]*[{[]/ { start = 1 }
    start { print }
  '
}

command_exists() {
	command -v "$1" >/dev/null 2>&1
}

command_in_path() {
	PATH="$ORIG_PATH" command -v "$1" >/dev/null 2>&1
}

path_contains() {
	printf '%s' "$ORIG_PATH" | tr ':' '\n' | grep -q "$1"
}

check_command() {
	local severity="$1"
	local label="$2"
	shift 2

	local output
	if output="$("$@" 2>&1)"; then
		ok "$label"
		return 0
	fi

	if [ "$severity" = "required" ]; then
		fail "$label"
	else
		warn "$label"
	fi
	note "$(compact_output "$output")"
	return 1
}

check_json_command() {
	local severity="$1"
	local label="$2"
	local filter="$3"
	shift 3

	local output
	if ! output="$("$@" 2>&1)"; then
		if [ "$severity" = "required" ]; then
			fail "$label"
		else
			warn "$label"
		fi
		note "$(compact_output "$output")"
		return 1
	fi

	if command_exists jq && printf '%s' "$output" | jq -e "$filter" >/dev/null 2>&1; then
		ok "$label"
		return 0
	fi

	if command_exists jq && extract_json "$output" | jq -e "$filter" >/dev/null 2>&1; then
		ok "$label"
		return 0
	fi

	if [ "$severity" = "required" ]; then
		fail "$label"
	else
		warn "$label"
	fi
	note "$(compact_output "$output")"
	return 1
}

check_port_selector() {
	local output
	local port

	if ! command_exists port-selector; then
		fail "port-selector installed"
		return 1
	fi

	if output="$(port-selector --name setup-check 2>&1)"; then
		port="$(printf '%s\n' "$output" | awk '/^[[:space:]]*[0-9]+[[:space:]]*$/ { gsub(/[[:space:]]/, "", $0); port=$0 } END { print port }')"
		if [ -n "$port" ]; then
			ok "port-selector returns a free port"
			port-selector --forget --name setup-check >/dev/null 2>&1 || true
			return 0
		fi
	fi

	fail "port-selector returns a free port"
	note "$(compact_output "$output")"
	port-selector --forget --name setup-check >/dev/null 2>&1 || true
	return 1
}

check_direnv_port() {
	local output

	if ! command_exists direnv; then
		fail "direnv installed"
		return 1
	fi

	if [ ! -f .envrc ]; then
		fail ".envrc present"
		return 1
	fi

	# shellcheck disable=SC2016
	if ! output="$(direnv exec . sh -lc 'printf "%s" "${PORT:-}"' 2>/dev/null)"; then
		fail "direnv exports numeric PORT"
		return 1
	fi

	if printf '%s' "$output" | tr -d '[:space:]' | grep -Eq '^[0-9]+$'; then
		ok "direnv exports numeric PORT"
		return 0
	fi

	fail "direnv exports numeric PORT"
	note "PORT=${output:-<empty>}"
	return 1
}

section "Shell integration (required)"
if command_in_path mise; then
	ok "mise in PATH"
else
	fail "mise in PATH"
	note "mise is not in your shell PATH. Install via brew or add ~/.local/bin to PATH."
fi
if path_contains "mise/shims\|mise/installs"; then
	ok "mise activate (tool paths in PATH)"
else
	fail "mise activate (tool paths in PATH)"
	note "mise tool directories not in PATH. Add 'mise activate' to your shell rc — see: https://mise.jdx.dev/getting-started.html"
fi
if command_in_path direnv; then
	ok "direnv in PATH"
else
	fail "direnv in PATH"
	note "direnv is not in your shell PATH. Ensure mise activate is configured so mise-installed tools are available."
fi
if [ -n "${DIRENV_DIR:-}" ]; then
	ok "direnv hook active (DIRENV_DIR is set)"
else
	fail "direnv hook active (DIRENV_DIR is set)"
	note "direnv hook is not configured in your shell. Add it — see: https://direnv.net/docs/hook.html"
fi

section "Core toolchain (required)"
check_command required "mise installed" mise --version
check_command required "direnv installed" direnv version
check_command required "gh installed" gh --version
check_command required "jq installed" jq --version
check_command required "node installed" node --version
check_command required "npx installed" npx --version
check_port_selector
check_direnv_port
check_command required "ruby installed" ruby --version

section "Agent CLIs (required)"
check_command required "claude installed" claude --version
check_command required "codex installed" codex --version
check_command required "playwright-cli installed" playwright-cli --version
check_command required "ccbox installed" ccbox --version
section "Account-backed tools"
check_json_command required "claude authenticated" '.loggedIn == true' claude auth status --json
check_command required "codex authenticated" codex login status
check_command required "gh authenticated" gh auth status

printf '\nSummary: %s required failure(s), %s warning(s)\n' "$required_failures" "$warnings"

if [ "$required_failures" -ne 0 ]; then
	exit 1
fi
