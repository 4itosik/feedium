#!/usr/bin/env bash

set -euo pipefail

PATH="${HOME}/.local/bin:/usr/local/bin:/opt/homebrew/bin:${PATH}"
export PATH

usage() {
	cat <<'EOF'
Usage: ./scripts/codex-context.sh [--session PATH]

Shows a Codex session summary:
- session metadata
- live token usage from the latest token_count event
- context window
- system/shared skills
- subagents spawned in the session
- rough baseline context estimate
EOF
}

die() {
	printf 'Error: %s\n' "$1" >&2
	exit 1
}

bytes_to_tokens() {
	echo $(($1 / 4))
}

extract_frontmatter() {
	awk '/^---$/ { if (n++) exit; next } n { print }' "$1"
}

find_latest_session() {
	find "${HOME}/.codex/sessions" -type f -name '*.jsonl' 2>/dev/null | LC_ALL=C sort | tail -n 1
}

join_lines() {
	awk '
		NF {
			if (seen++) {
				printf ", "
			}
			printf "%s", $0
		}
		END {
			if (!seen) {
				printf "none"
			}
		}
	'
}

session_file=""

while [ $# -gt 0 ]; do
	case "$1" in
	--session)
		shift
		[ $# -gt 0 ] || die "--session requires a path"
		session_file="$1"
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		die "unknown argument: $1"
		;;
	esac
	shift
done

command -v jq >/dev/null 2>&1 || die "jq is required"

if [ -z "$session_file" ]; then
	session_file="$(find_latest_session)"
fi

[ -n "$session_file" ] || die "no Codex session logs found in ~/.codex/sessions"
[ -f "$session_file" ] || die "session file not found: $session_file"

session_meta_json="$(jq -sc 'map(select(.type == "session_meta")) | last' "$session_file")"
[ "$session_meta_json" != "null" ] || die "session_meta not found in $session_file"

turn_context_json="$(jq -sc 'map(select(.type == "turn_context")) | last' "$session_file")"
token_count_json="$(jq -sc 'map(select(.type == "event_msg" and .payload.type == "token_count" and .payload.info != null)) | last' "$session_file")"

started_at="$(printf '%s' "$session_meta_json" | jq -r '.payload.timestamp // "unknown"')"
cwd="$(printf '%s' "$session_meta_json" | jq -r '.payload.cwd // "unknown"')"
cli_version="$(printf '%s' "$session_meta_json" | jq -r '.payload.cli_version // "unknown"')"
base_model="$(printf '%s' "$session_meta_json" | jq -r '.payload.model // empty')"
base_instructions_chars="$(printf '%s' "$session_meta_json" | jq -r '(.payload.base_instructions.text // "") | length')"

if [ "$turn_context_json" = "null" ]; then
	model="$base_model"
	reasoning_effort="unknown"
	sandbox_policy="unknown"
	approval_policy="unknown"
else
	model="$(printf '%s' "$turn_context_json" | jq -r '.payload.model // "'"$base_model"'" // "unknown"')"
	reasoning_effort="$(printf '%s' "$turn_context_json" | jq -r '.payload.effort // "unknown"')"
	sandbox_policy="$(printf '%s' "$turn_context_json" | jq -r '.payload.sandbox_policy.type // "unknown"')"
	approval_policy="$(printf '%s' "$turn_context_json" | jq -r '.payload.approval_policy // "unknown"')"
fi

developer_chars="$(jq -sr '[.[] | select(.type == "response_item" and .payload.type == "message" and .payload.role == "developer") | [(.payload.content[]? | .text // empty)] | join("\n")] | join("\n") | length' "$session_file")"
latest_user_chars="$(jq -sr '[.[] | select(.type == "event_msg" and .payload.type == "user_message") | .payload.message] | if length == 0 then 0 else (last | length) end' "$session_file")"

total_input_tokens="n/a"
total_cached_tokens="n/a"
total_output_tokens="n/a"
total_reasoning_tokens="n/a"
total_tokens="n/a"
last_input_tokens="n/a"
last_cached_tokens="n/a"
last_output_tokens="n/a"
last_reasoning_tokens="n/a"
last_total_tokens="n/a"
context_window="n/a"

if [ "$token_count_json" != "null" ]; then
	total_input_tokens="$(printf '%s' "$token_count_json" | jq -r '.payload.info.total_token_usage.input_tokens // "n/a"')"
	total_cached_tokens="$(printf '%s' "$token_count_json" | jq -r '.payload.info.total_token_usage.cached_input_tokens // "n/a"')"
	total_output_tokens="$(printf '%s' "$token_count_json" | jq -r '.payload.info.total_token_usage.output_tokens // "n/a"')"
	total_reasoning_tokens="$(printf '%s' "$token_count_json" | jq -r '.payload.info.total_token_usage.reasoning_output_tokens // "n/a"')"
	total_tokens="$(printf '%s' "$token_count_json" | jq -r '.payload.info.total_token_usage.total_tokens // "n/a"')"
	last_input_tokens="$(printf '%s' "$token_count_json" | jq -r '.payload.info.last_token_usage.input_tokens // "n/a"')"
	last_cached_tokens="$(printf '%s' "$token_count_json" | jq -r '.payload.info.last_token_usage.cached_input_tokens // "n/a"')"
	last_output_tokens="$(printf '%s' "$token_count_json" | jq -r '.payload.info.last_token_usage.output_tokens // "n/a"')"
	last_reasoning_tokens="$(printf '%s' "$token_count_json" | jq -r '.payload.info.last_token_usage.reasoning_output_tokens // "n/a"')"
	last_total_tokens="$(printf '%s' "$token_count_json" | jq -r '.payload.info.last_token_usage.total_tokens // "n/a"')"
	context_window="$(printf '%s' "$token_count_json" | jq -r '.payload.info.model_context_window // "n/a"')"
fi

system_skills=""
if [ -d "${HOME}/.codex/skills/.system" ]; then
	system_skills="$(find "${HOME}/.codex/skills/.system" -mindepth 1 -maxdepth 1 -type d -exec basename {} \; | LC_ALL=C sort)"
fi

shared_skills=""
if [ -d "${HOME}/.agents/skills" ]; then
	shared_skills="$(find "${HOME}/.agents/skills" -mindepth 1 -maxdepth 1 -type d -exec basename {} \; | LC_ALL=C sort)"
fi

system_skill_count="$(printf '%s\n' "$system_skills" | sed '/^$/d' | wc -l | tr -d ' ')"
shared_skill_count="$(printf '%s\n' "$shared_skills" | sed '/^$/d' | wc -l | tr -d ' ')"

skills_bytes=0
for d in "${HOME}"/.agents/skills/*/ "${HOME}"/.codex/skills/.system/*/; do
	[ -d "$d" ] || continue
	f="${d}SKILL.md"
	[ -f "$f" ] || continue
	b=$(extract_frontmatter "$f" | wc -c | tr -d ' ')
	skills_bytes=$((skills_bytes + b))
done

codex_agents_md_bytes=0
if [ -f "${HOME}/.codex/AGENTS.md" ]; then
	codex_agents_md_bytes="$(wc -c <"${HOME}/.codex/AGENTS.md" | tr -d ' ')"
fi

spawned_agents_json="$(jq -sr '
	def parse_args: (.payload.arguments | fromjson? // {});
	def parse_output:
		(.payload.output | fromjson? // {});
	[ .[] | select(.type == "response_item" and .payload.type == "function_call" and .payload.name == "spawn_agent")
		| {call_id: .payload.call_id, args: parse_args}
	] as $calls
	| [ .[] | select(.type == "response_item" and .payload.type == "function_call_output")
		| {call_id: .payload.call_id, out: parse_output}
	] as $outputs
	| [ $calls[]
		| . as $call
		| ($outputs[]? | select(.call_id == $call.call_id) | .out) as $out
		| {
			call_id: $call.call_id,
			agent_type: ($call.args.agent_type // "default"),
			fork_context: ($call.args.fork_context // false),
			agent_id: ($out.agent_id // "unknown"),
			nickname: ($out.nickname // "unknown")
		}
	]' "$session_file")"

spawned_count="$(printf '%s' "$spawned_agents_json" | jq 'length')"
send_input_count="$(jq -sr '[.[] | select(.type == "response_item" and .payload.type == "function_call" and .payload.name == "send_input")] | length' "$session_file")"
wait_agent_count="$(jq -sr '[.[] | select(.type == "response_item" and .payload.type == "function_call" and .payload.name == "wait_agent")] | length' "$session_file")"
close_agent_count="$(jq -sr '[.[] | select(.type == "response_item" and .payload.type == "function_call" and .payload.name == "close_agent")] | length' "$session_file")"
resume_agent_count="$(jq -sr '[.[] | select(.type == "response_item" and .payload.type == "function_call" and .payload.name == "resume_agent")] | length' "$session_file")"

printf '== Session ==\n'
printf 'session:   %s\n' "$session_file"
printf 'started:   %s\n' "$started_at"
printf 'cwd:       %s\n' "$cwd"
printf 'model:     %s\n' "${model:-unknown}"
printf 'reasoning: %s\n' "$reasoning_effort"
printf 'cli:       %s\n' "$cli_version"
printf 'sandbox:   %s\n' "$sandbox_policy"
printf 'approval:  %s\n' "$approval_policy"

printf '\n== Live Usage ==\n'
printf 'context window: %s tokens\n' "$context_window"
printf 'session total:  input=%s cached=%s output=%s reasoning=%s total=%s\n' \
	"$total_input_tokens" "$total_cached_tokens" "$total_output_tokens" "$total_reasoning_tokens" "$total_tokens"
printf 'last turn:      input=%s cached=%s output=%s reasoning=%s total=%s\n' \
	"$last_input_tokens" "$last_cached_tokens" "$last_output_tokens" "$last_reasoning_tokens" "$last_total_tokens"
printf 'note: exact context remaining is not exposed in session logs; use Codex TUI status_line for that\n'

printf '\n== Skills ==\n'
printf 'system skills (%s): %s\n' "$system_skill_count" "$(printf '%s\n' "$system_skills" | join_lines)"
printf 'shared skills (%s): %s\n' "$shared_skill_count" "$(printf '%s\n' "$shared_skills" | join_lines)"
if [ "$codex_agents_md_bytes" -gt 0 ]; then
	printf 'AGENTS.md: present (~%s tokens)\n' "$(bytes_to_tokens "$codex_agents_md_bytes")"
else
	printf 'AGENTS.md: absent\n'
fi

printf '\n== Subagents ==\n'
printf 'spawned: %s\n' "$spawned_count"
printf 'activity: send_input=%s wait_agent=%s resume_agent=%s close_agent=%s\n' \
	"$send_input_count" "$wait_agent_count" "$resume_agent_count" "$close_agent_count"
if [ "$spawned_count" -gt 0 ]; then
	printf '%s' "$spawned_agents_json" | jq -r '.[] | "- \(.nickname) \(.agent_id) type=\(.agent_type) fork_context=\(.fork_context)"'
fi

printf '\n== Baseline Estimate ==\n'
printf 'method: rough estimate, ~4 chars/token\n'
printf 'base instructions: ~%s tokens\n' "$(bytes_to_tokens "$base_instructions_chars")"
printf 'developer overlay: ~%s tokens\n' "$(bytes_to_tokens "$developer_chars")"
printf 'skill metadata:    ~%s tokens\n' "$(bytes_to_tokens "$skills_bytes")"
printf 'AGENTS.md:         ~%s tokens\n' "$(bytes_to_tokens "$codex_agents_md_bytes")"
printf 'latest user msg:   ~%s tokens\n' "$(bytes_to_tokens "$latest_user_chars")"
