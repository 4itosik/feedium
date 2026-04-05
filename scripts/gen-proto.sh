#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/.." && pwd)"

if ! command -v protoc >/dev/null 2>&1; then
  echo "error: protoc is not installed or not in PATH" >&2
  exit 1
fi

if ! command -v protoc-gen-go >/dev/null 2>&1; then
  echo "error: protoc-gen-go is not installed or not in PATH" >&2
  exit 1
fi

if ! command -v protoc-gen-connect-go >/dev/null 2>&1; then
  echo "error: protoc-gen-connect-go is not installed or not in PATH" >&2
  exit 1
fi

detect_protoc_include() {
  if [[ -n "${PROTOC_INCLUDE:-}" ]]; then
    if [[ -f "${PROTOC_INCLUDE}/google/protobuf/timestamp.proto" ]]; then
      echo "${PROTOC_INCLUDE}"
      return 0
    fi
    echo "error: PROTOC_INCLUDE is set, but timestamp.proto was not found there: ${PROTOC_INCLUDE}" >&2
    return 1
  fi

  local candidates=()
  local protoc_bin
  protoc_bin="$(command -v protoc)"
  candidates+=("$(cd "$(dirname "${protoc_bin}")/.." && pwd)/include")
  candidates+=("/opt/homebrew/include" "/usr/local/include" "/usr/include")

  if command -v brew >/dev/null 2>&1; then
    local brew_prefix
    brew_prefix="$(brew --prefix protobuf 2>/dev/null || true)"
    if [[ -n "${brew_prefix}" ]]; then
      candidates+=("${brew_prefix}/include")
    fi
  fi

  local dir
  for dir in "${candidates[@]}"; do
    if [[ -f "${dir}/google/protobuf/timestamp.proto" ]]; then
      echo "${dir}"
      return 0
    fi
  done

  echo "error: failed to find protobuf include dir; set PROTOC_INCLUDE explicitly" >&2
  return 1
}

protoc_include="$(detect_protoc_include)"

cd "${repo_root}"
protoc \
  -I . \
  -I "${protoc_include}" \
  --go_out=. \
  --go_opt=module=feedium \
  --connect-go_out=. \
  --connect-go_opt=module=feedium \
  api/source/v1/source.proto
