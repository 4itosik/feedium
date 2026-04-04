#!/usr/bin/env bash

# Verify that Claude/Codex context is clean at startup:
# no unexpected skills, MCP servers, plugins, agents, or marketplaces.

set -u

PATH="${HOME}/.local/bin:/usr/local/bin:/opt/homebrew/bin:${PATH}"
export PATH

failures=0
warnings=0

# --- Expected items (override via env vars) ---

EXPECTED_SKILLS="${EXPECTED_SKILLS:-docmost fpf-problem-solving gws-calendar gws-calendar-agenda gws-calendar-insert gws-docs gws-docs-write gws-drive gws-drive-upload gws-gmail gws-gmail-forward gws-gmail-reply gws-gmail-reply-all gws-gmail-send gws-gmail-triage gws-meet gws-sheets gws-tasks playwright-cli prompt-engineering tgcli}"

# Optional items — allowed but not required (present = OK, absent = OK)
OPTIONAL_SKILLS="${OPTIONAL_SKILLS:-ccbox ccbox-insights}"

EXPECTED_MCP="${EXPECTED_MCP:-tavily context7}"

EXPECTED_PLUGINS="${EXPECTED_PLUGINS:-}"

EXPECTED_MARKETPLACES="${EXPECTED_MARKETPLACES:-claude-plugins-official dapi playwright-skill}"

EXPECTED_AGENTS="${EXPECTED_AGENTS:-backend-architect business-panel-experts deep-research-agent devops-architect frontend-architect learning-guide performance-engineer python-expert quality-engineer refactoring-expert requirements-analyst root-cause-analyst security-engineer socratic-mentor system-architect technical-writer}"

# --- Helpers ---

section() {
	printf '\n== %s ==\n' "$1"
}

ok() {
	printf '\033[32m[OK]\033[0m   %s\n' "$1"
}

fail() {
	printf '\033[31m[FAIL]\033[0m %s\n' "$1"
	failures=$((failures + 1))
}

to_sorted_lines() {
	# shellcheck disable=SC2086 # intentional word splitting
	printf '%s\n' $1 | sort
}

diff_lists() {
	local label="$1"
	local expected="$2"
	local actual="$3"
	local optional="${4:-}"

	local extra
	extra="$(comm -23 <(to_sorted_lines "$actual") <(to_sorted_lines "$expected"))"

	# Remove optional items from the "extra" list — they are allowed but not required
	if [ -n "$optional" ] && [ -n "$extra" ]; then
		extra="$(comm -23 <(printf '%s\n' "$extra" | sort) <(to_sorted_lines "$optional"))"
	fi

	local missing
	missing="$(comm -13 <(to_sorted_lines "$actual") <(to_sorted_lines "$expected"))"

	if [ -n "$extra" ]; then
		fail "unexpected $label:"
		printf '%s\n' "$extra" | while IFS= read -r item; do
			printf '       + %s\n' "$item"
		done
	fi

	if [ -n "$missing" ]; then
		fail "missing $label:"
		printf '%s\n' "$missing" | while IFS= read -r item; do
			printf '       - %s\n' "$item"
		done
	fi

	if [ -z "$extra" ] && [ -z "$missing" ]; then
		# shellcheck disable=SC2086 # intentional word splitting
		ok "$label clean ($(printf '%s\n' $expected | wc -l | tr -d ' ') items)"
	fi
}

# --- Skills ---

section "Skills (global)"

if command -v npx >/dev/null 2>&1 && command -v jq >/dev/null 2>&1; then
	actual_skills="$(npx skills ls -g --json 2>/dev/null | jq -r '.[].name' | sort | tr '\n' ' ')"
	diff_lists "skills" "$EXPECTED_SKILLS" "$actual_skills" "$OPTIONAL_SKILLS"
else
	fail "npx or jq not available for skills check"
fi

# --- MCP servers ---

section "MCP servers (Claude)"

if command -v claude >/dev/null 2>&1; then
	# Parse "name: command - status" lines; name may contain colons (e.g. plugin:playwright:playwright)
	actual_mcp="$(claude mcp list 2>&1 | grep -E '^[a-zA-Z].*: .* - ' | sed 's/: .*//' | sort | tr '\n' ' ')"
	diff_lists "MCP servers" "$EXPECTED_MCP" "$actual_mcp"
else
	fail "claude not available for MCP check"
fi

# --- Claude plugins ---

section "Plugins (Claude)"

if command -v claude >/dev/null 2>&1; then
	actual_plugins="$(claude plugins list 2>&1 | grep -E '^\s+❯' | sed 's/.*❯ //; s/[[:space:]]*$//' | sort | tr '\n' ' ')"
	diff_lists "plugins" "$EXPECTED_PLUGINS" "$actual_plugins"
else
	fail "claude not available for plugins check"
fi

# --- Claude marketplaces ---

section "Marketplaces (Claude)"

if command -v claude >/dev/null 2>&1; then
	actual_mp="$(claude plugins marketplace list 2>&1 | grep -E '^\s+❯' | sed 's/.*❯ //; s/[[:space:]]*$//' | sort | tr '\n' ' ')"
	diff_lists "marketplaces" "$EXPECTED_MARKETPLACES" "$actual_mp"
else
	fail "claude not available for marketplace check"
fi

# --- Claude agents ---

section "Agents (Claude)"

agents_dir="${HOME}/.claude/agents"
if [ -d "$agents_dir" ]; then
	actual_agents="$(find "$agents_dir" -maxdepth 1 -name '*.md' -exec basename {} .md \; | sort | tr '\n' ' ')"
	diff_lists "agents" "$EXPECTED_AGENTS" "$actual_agents"
else
	fail "agents dir not found: $agents_dir"
fi

# --- Codex extras ---

section "Codex extras"

codex_skills_dir="${HOME}/.codex/skills"
if [ -d "$codex_skills_dir" ]; then
	actual_codex_skills=""
	for f in "$codex_skills_dir"/*; do
		[ -e "$f" ] || continue
		name="$(basename "$f")"
		case "$name" in .*) continue ;; esac
		actual_codex_skills="${actual_codex_skills} ${name}"
	done
	if [ -n "$actual_codex_skills" ]; then
		fail "unexpected Codex skills:"
		# shellcheck disable=SC2086 # intentional word splitting
		for s in $actual_codex_skills; do
			printf '       + %s\n' "$s"
		done
	else
		ok "Codex skills dir empty"
	fi
else
	ok "Codex skills dir does not exist"
fi

codex_memories_dir="${HOME}/.codex/memories"
if [ -d "$codex_memories_dir" ]; then
	actual_codex_memories=""
	for f in "$codex_memories_dir"/*; do
		[ -e "$f" ] || continue
		name="$(basename "$f")"
		case "$name" in .*) continue ;; esac
		actual_codex_memories="${actual_codex_memories} ${name}"
	done
	if [ -n "$actual_codex_memories" ]; then
		fail "unexpected Codex memories:"
		# shellcheck disable=SC2086 # intentional word splitting
		for m in $actual_codex_memories; do
			printf '       + %s\n' "$m"
		done
	else
		ok "Codex memories dir empty"
	fi
else
	ok "Codex memories dir does not exist"
fi

codex_agents_md="${HOME}/.codex/AGENTS.md"
if [ -f "$codex_agents_md" ]; then
	b=$(wc -c <"$codex_agents_md" | tr -d ' ')
	printf '\033[34m[INFO]\033[0m Codex AGENTS.md exists (%s bytes)\n' "$b"
else
	ok "Codex AGENTS.md does not exist"
fi

# --- Context size estimate ---
#
# Claude Code loads only frontmatter metadata (name + description) at startup,
# not full file contents. Full content is loaded on-demand when invoked.
# We extract frontmatter from YAML --- blocks to estimate actual startup cost.

section "Context size (tokens estimate)"

CONTEXT_WARN_TOKENS="${CONTEXT_WARN_TOKENS:-5000}"
CONTEXT_FAIL_TOKENS="${CONTEXT_FAIL_TOKENS:-10000}"

# Extract YAML frontmatter between --- markers
extract_frontmatter() {
	awk '/^---$/ { if (n++) exit; next } n { print }' "$1"
}

# Rough token estimate: ~4 chars per token
bytes_to_tokens() {
	echo $(($1 / 4))
}

agents_bytes=0
if [ -d "${HOME}/.claude/agents" ]; then
	for f in "${HOME}/.claude/agents"/*.md; do
		[ -f "$f" ] || continue
		b=$(extract_frontmatter "$f" | wc -c | tr -d ' ')
		agents_bytes=$((agents_bytes + b))
	done
fi

skills_bytes=0
if [ -d "${HOME}/.agents/skills" ]; then
	for d in "${HOME}/.agents/skills"/*/; do
		[ -d "$d" ] || continue
		f="${d}SKILL.md"
		[ -f "$f" ] || continue
		b=$(extract_frontmatter "$f" | wc -c | tr -d ' ')
		skills_bytes=$((skills_bytes + b))
	done
fi

# Build list of enabled plugins from settings.json
enabled_plugin_dirs=""
settings_file="${HOME}/.claude/settings.json"
if [ -f "$settings_file" ] && command -v jq >/dev/null 2>&1; then
	enabled_list="$(jq -r '.enabledPlugins // {} | to_entries[] | select(.value == true) | .key' "$settings_file" 2>/dev/null)"
	while IFS= read -r entry; do
		[ -z "$entry" ] && continue
		plugin_name="${entry%%@*}"
		marketplace="${entry#*@}"
		version_dir="$(find "${HOME}/.claude/plugins/cache/${marketplace}/${plugin_name}" -mindepth 1 -maxdepth 1 -type d 2>/dev/null | sort -V | tail -1)"
		if [ -n "$version_dir" ]; then
			enabled_plugin_dirs="${enabled_plugin_dirs} ${version_dir}"
		fi
	done <<<"$enabled_list"
fi

plugins_bytes=0
# shellcheck disable=SC2086 # intentional word splitting on enabled_plugin_dirs
for pd in $enabled_plugin_dirs; do
	while IFS= read -r sf; do
		[ -n "$sf" ] || continue
		b=$(extract_frontmatter "$sf" | wc -c | tr -d ' ')
		plugins_bytes=$((plugins_bytes + b))
	done < <(find "$pd" -name 'SKILL.md' 2>/dev/null)
done

# Codex-specific: AGENTS.md is loaded into context (rules are NOT — they're CLI permissions)
codex_agents_md_bytes=0
if [ -f "${HOME}/.codex/AGENTS.md" ]; then
	codex_agents_md_bytes="$(wc -c <"${HOME}/.codex/AGENTS.md" | tr -d ' ')"
fi

# --- Claude totals ---

claude_total=$((agents_bytes + skills_bytes + plugins_bytes))
claude_tokens=$(bytes_to_tokens "$claude_total")

printf '\n  Claude Code:\n'
printf '    agents:  ~%s tokens (%s items)\n' "$(bytes_to_tokens "$agents_bytes")" "$(find "${HOME}/.claude/agents" -maxdepth 1 -name '*.md' 2>/dev/null | wc -l | tr -d ' ')"
printf '    skills:  ~%s tokens (%s items)\n' "$(bytes_to_tokens "$skills_bytes")" "$(find "${HOME}/.agents/skills" -maxdepth 1 -mindepth 1 -type d 2>/dev/null | wc -l | tr -d ' ')"
printf '    plugins: ~%s tokens (enabled)\n' "$(bytes_to_tokens "$plugins_bytes")"
printf '    total:   ~%s tokens\n' "$claude_tokens"

# --- Codex totals ---
# Skills are shared (same ~/.agents/skills/), so count skills_bytes for Codex too

codex_total=$((skills_bytes + codex_agents_md_bytes))
codex_tokens=$(bytes_to_tokens "$codex_total")

printf '\n  Codex:\n'
printf '    skills:    ~%s tokens (shared)\n' "$(bytes_to_tokens "$skills_bytes")"
printf '    AGENTS.md: ~%s tokens\n' "$(bytes_to_tokens "$codex_agents_md_bytes")"
printf '    total:     ~%s tokens\n' "$codex_tokens"

# --- Thresholds (applied per-agent) ---

printf '\n'

check_threshold() {
	local label="$1"
	local tokens="$2"

	if [ "$tokens" -ge "$CONTEXT_FAIL_TOKENS" ]; then
		fail "$label baseline ~${tokens} tokens (threshold: ${CONTEXT_FAIL_TOKENS})"
	elif [ "$tokens" -ge "$CONTEXT_WARN_TOKENS" ]; then
		printf '\033[33m[WARN]\033[0m %s baseline ~%s tokens (threshold: %s)\n' "$label" "$tokens" "$CONTEXT_WARN_TOKENS"
		warnings=$((warnings + 1))
	else
		ok "$label baseline ~${tokens} tokens"
	fi
}

check_threshold "Claude" "$claude_tokens"
check_threshold "Codex" "$codex_tokens"

# --- Top consumers (combined) ---

printf '\n  Top consumers:\n'

{
	for f in "${HOME}/.claude/agents"/*.md; do
		[ -f "$f" ] || continue
		b=$(extract_frontmatter "$f" | wc -c | tr -d ' ')
		t=$(bytes_to_tokens "$b")
		printf '%s\tagent:%s\n' "$t" "$(basename "$f" .md)"
	done

	for d in "${HOME}/.agents/skills"/*/; do
		[ -d "$d" ] || continue
		f="${d}SKILL.md"
		[ -f "$f" ] || continue
		b=$(extract_frontmatter "$f" | wc -c | tr -d ' ')
		t=$(bytes_to_tokens "$b")
		printf '%s\tskill:%s\n' "$t" "$(basename "$d")"
	done

	# shellcheck disable=SC2086 # intentional word splitting on enabled_plugin_dirs
	for pd in $enabled_plugin_dirs; do
		while IFS= read -r sf; do
			[ -n "$sf" ] || continue
			b=$(extract_frontmatter "$sf" | wc -c | tr -d ' ')
			[ "$b" -gt 0 ] || continue
			t=$(bytes_to_tokens "$b")
			version_dir="$pd"
			name=$(basename "$(dirname "$version_dir")")
			mp=$(basename "$(dirname "$(dirname "$version_dir")")")
			printf '%s\tplugin:%s@%s\n' "$t" "$name" "$mp"
		done < <(find "$pd" -name 'SKILL.md' 2>/dev/null)
	done

	if [ "$codex_agents_md_bytes" -gt 0 ]; then
		printf '%s\tcodex:AGENTS.md\n' "$(bytes_to_tokens "$codex_agents_md_bytes")"
	fi
} | sort -rn | head -10 | while IFS=$'\t' read -r tokens name; do
	printf '    %6s tokens  %s\n' "$tokens" "$name"
done

# --- Summary ---

printf '\nContext check: %s failure(s)\n' "$failures"

if [ "$failures" -ne 0 ]; then
	exit 1
fi
