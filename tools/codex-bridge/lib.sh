#!/usr/bin/env bash
# Shared helpers for claude-bridge.sh and codex-server.sh. Errors go to stderr, so stdout
# carries only the command's own answer.

# die prints an error message to stderr and exits.
die() {
  printf 'erro: %s\n' "$*" >&2
  exit 1
}

# file_mode prints the octal permission bits of a file on Linux and macOS.
file_mode() {
  stat -c '%a' "$1" 2>/dev/null || stat -f '%Lp' "$1"
}

# file_owner prints the numeric owner id of a file on Linux and macOS.
file_owner() {
  stat -c '%u' "$1" 2>/dev/null || stat -f '%u' "$1"
}

# load_env exports KEY=VALUE lines from a private .env file without executing it.
load_env() {
  local file="$1" line key value
  [[ -f "$file" ]] || die "arquivo $file não encontrado. Copie .env.example para .env, preencha e rode chmod 600."
  [[ "$(file_mode "$file")" == "600" ]] || die "o arquivo $file precisa ter permissão 600. Rode: chmod 600 $file"
  [[ "$(file_owner "$file")" == "$(id -u)" ]] || die "o arquivo $file precisa pertencer ao usuário atual."
  while IFS= read -r line || [[ -n "$line" ]]; do
    line="${line%$'\r'}"
    [[ "$line" =~ ^[[:space:]]*(#|$) ]] && continue
    [[ "$line" =~ ^([A-Z][A-Z0-9_]*)=(.*)$ ]] || die "linha inválida em $file: $line"
    key="${BASH_REMATCH[1]}"
    value="${BASH_REMATCH[2]}"
    if [[ "$value" =~ ^\"(.*)\"$ || "$value" =~ ^\'(.*)\'$ ]]; then
      value="${BASH_REMATCH[1]}"
    fi
    export "$key=$value"
  done <"$file"
}

# require_vars fails when any of the named variables is empty.
require_vars() {
  local name
  for name in "$@"; do
    [[ -n "${!name:-}" ]] || die "defina $name no .env"
  done
}

# expand_home replaces a leading ~ in a local path with the home directory.
expand_home() {
  local path="$1"
  # The literal tilde is what we are matching, so it must stay quoted.
  # shellcheck disable=SC2088
  if [[ "$path" == "~" || "$path" == "~/"* ]]; then
    path="$HOME${path:1}"
  fi
  printf '%s' "$path"
}
