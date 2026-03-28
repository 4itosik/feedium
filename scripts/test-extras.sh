#!/usr/bin/env bash

set -u

PATH="${HOME}/.local/bin:/usr/local/bin:/opt/homebrew/bin:${PATH}"
export PATH

warnings=0

REQUIRED_SKILLS=(
	"tgcli"
	"playwright-cli"
	"prompt-engineering"
	"gws-docs"
	"gws-docs-write"
	"gws-drive"
	"gws-sheets"
)

DEFAULT_CLAUDE_MARKETPLACE_NAMES="${CLAUDE_MARKETPLACE_NAMES:-dapi}"
DEFAULT_CLAUDE_EXPECTED_PLUGINS="${CLAUDE_EXPECTED_PLUGINS:-himalaya@dapi pr-review-fix-loop@dapi spec-reviewer@dapi zellij-workflow@dapi}"

read -r -a OPTIONAL_CLAUDE_MARKETPLACES <<<"$DEFAULT_CLAUDE_MARKETPLACE_NAMES"
read -r -a OPTIONAL_CLAUDE_PLUGINS <<<"$DEFAULT_CLAUDE_EXPECTED_PLUGINS"

section() {
	printf '\n== %s ==\n' "$1"
}

ok() {
	printf '\033[32m[OK]\033[0m %s\n' "$1"
}

warn() {
	printf '\033[33m[WARN]\033[0m %s\n' "$1"
	warnings=$((warnings + 1))
}

fail() {
	printf '\033[31m[FAIL]\033[0m %s\n' "$1"
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

command_exists() {
	command -v "$1" >/dev/null 2>&1
}

check_command() {
	local label="$1"
	shift

	local output
	if output="$("$@" 2>&1)"; then
		ok "$label"
		return 0
	fi

	warn "$label"
	note "$(compact_output "$output")"
	return 1
}

check_json_command() {
	local label="$1"
	local filter="$2"
	shift 2

	local output
	if ! output="$("$@" 2>&1)"; then
		warn "$label"
		note "$(compact_output "$output")"
		return 1
	fi

	local json
	json="$(printf '%s\n' "$output" | awk '
    BEGIN { start = 0 }
    /^[[:space:]]*[{[]/ { start = 1 }
    start { print }
  ')"

	if command_exists jq; then
		if printf '%s' "$output" | jq -e "$filter" >/dev/null 2>&1; then
			ok "$label"
			return 0
		fi
		if [ -n "$json" ] && printf '%s' "$json" | jq -e "$filter" >/dev/null 2>&1; then
			ok "$label"
			return 0
		fi
	fi

	warn "$label"
	note "$(compact_output "$output")"
	return 1
}

check_himalaya_accounts() {
	local output

	if ! command_exists himalaya; then
		warn "himalaya installed"
		return 1
	fi

	if ! output="$(himalaya account list --output json 2>&1)"; then
		warn "himalaya mail account configured"
		note "$(compact_output "$output")"
		return 1
	fi

	if command_exists jq && printf '%s' "$output" | jq -e 'length > 0' >/dev/null 2>&1; then
		ok "himalaya mail account configured"
		return 0
	fi

	warn "himalaya mail account configured"
	note "$(compact_output "$output")"
	return 1
}

check_skills() {
	local output
	local skill

	if ! command_exists npx; then
		warn "npx available for skills checks"
		return 1
	fi

	if ! command_exists jq; then
		warn "jq available for skills checks"
		return 1
	fi

	if ! output="$(npx skills ls -g --json 2>&1)"; then
		warn "curated skills can be listed"
		note "$(compact_output "$output")"
		return 1
	fi

	for skill in "${REQUIRED_SKILLS[@]}"; do
		if printf '%s' "$output" | jq -e --arg skill "$skill" 'map(select(.name == $skill)) | length > 0' >/dev/null 2>&1; then
			ok "skill installed: $skill"
		else
			warn "skill installed: $skill"
		fi
	done
}

check_claude_plugins() {
	local marketplaces_output
	local plugins_output
	local name

	if ! command_exists claude; then
		warn "claude installed"
		return 1
	fi

	if ! marketplaces_output="$(claude plugins marketplace list 2>&1)"; then
		warn "Claude plugin marketplaces can be listed"
		note "$(compact_output "$marketplaces_output")"
		return 1
	fi

	for name in "${OPTIONAL_CLAUDE_MARKETPLACES[@]}"; do
		if printf '%s' "$marketplaces_output" | grep -Fq "$name"; then
			ok "Claude marketplace present: $name"
		else
			warn "Claude marketplace present: $name"
		fi
	done

	if ! plugins_output="$(claude plugins list 2>&1)"; then
		warn "Claude plugins can be listed"
		note "$(compact_output "$plugins_output")"
		return 1
	fi

	for name in "${OPTIONAL_CLAUDE_PLUGINS[@]}"; do
		if printf '%s' "$plugins_output" | grep -Fq "$name"; then
			ok "Claude plugin installed: $name"
		else
			warn "Claude plugin installed: $name"
		fi
	done
}

section "Optional agent extras"
check_command "tgcli installed" tgcli --help
check_command "gws installed" gws --help
check_command "himalaya installed" himalaya --version
check_command "tgcli authenticated" tgcli auth status
check_json_command "gws authenticated" '(.token_valid // false) == true' gws auth status
check_himalaya_accounts
check_skills
check_claude_plugins

printf '\nExtras summary: %s warning(s)\n' "$warnings"
