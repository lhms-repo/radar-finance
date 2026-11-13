#!/usr/bin/env bash
# Runs on the Codex machine: runs one Codex task non-interactively inside the project, with
# settings from a private .env. claude-bridge.sh calls it over SSH and sends the prompt on stdin.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source-path=SCRIPTDIR
source "$script_dir/lib.sh"

load_env "${CODEX_BRIDGE_ENV:-$script_dir/.env}"

CODEX_REPO_DIR="$(expand_home "${CODEX_REPO_DIR:-$(cd "$script_dir/../.." && pwd)}")"
CODEX_BIN="$(expand_home "${CODEX_BIN:-codex}")"
CODEX_SANDBOX="${CODEX_SANDBOX:-workspace-write}"
CODEX_MODEL="${CODEX_MODEL:-}"
if [[ -n "${CODEX_PATH_EXTRA:-}" ]]; then
  export PATH="$CODEX_PATH_EXTRA:$PATH"
fi
if [[ -n "${CODEX_HOME_DIR:-}" ]]; then
  CODEX_HOME="$(expand_home "$CODEX_HOME_DIR")"
  export CODEX_HOME
fi

[[ -d "$CODEX_REPO_DIR" ]] || die "CODEX_REPO_DIR não existe: $CODEX_REPO_DIR"
[[ -f "$CODEX_REPO_DIR/AGENTS.md" ]] || die "AGENTS.md não encontrado em $CODEX_REPO_DIR"
command -v "$CODEX_BIN" >/dev/null 2>&1 || die "Codex não encontrado: $CODEX_BIN. Ajuste CODEX_BIN ou CODEX_PATH_EXTRA."
case "$CODEX_SANDBOX" in
  read-only | workspace-write) ;;
  *) die "CODEX_SANDBOX aceita só read-only ou workspace-write" ;;
esac
cd "$CODEX_REPO_DIR"

# print_check reports the resolved settings and tool versions, for a manual test over SSH.
print_check() {
  printf 'pasta do projeto: %s\n' "$CODEX_REPO_DIR"
  printf 'codex: %s (%s)\n' "$(command -v "$CODEX_BIN")" "$("$CODEX_BIN" --version 2>/dev/null | head -n 1)"
  printf 'login do codex: %s\n' "$("$CODEX_BIN" login status 2>&1 | tail -n 1)"
  printf 'CODEX_HOME: %s\n' "${CODEX_HOME:-padrão (~/.codex)}"
  printf 'sandbox: %s\n' "$CODEX_SANDBOX"
  printf 'modelo: %s\n' "${CODEX_MODEL:-padrão da configuração do Codex}"
  local tool
  for tool in go pnpm node make git flock; do
    if command -v "$tool" >/dev/null 2>&1; then
      printf '%s: ok\n' "$tool"
    else
      printf '%s: não encontrado no PATH, ajuste CODEX_PATH_EXTRA\n' "$tool"
    fi
  done
}

# acquire_lock makes sure only one Codex task runs on this machine at a time.
acquire_lock() {
  if ! command -v flock >/dev/null 2>&1; then
    return 0
  fi
  exec 9>"${XDG_RUNTIME_DIR:-/tmp}/radar-codex-bridge.lock"
  flock -n 9 || die "já existe uma tarefa do Codex em andamento nesta máquina"
}

# build_codex_args fills CODEX_ARGS with the options shared by new and resumed tasks.
build_codex_args() {
  CODEX_ARGS=(--json -c "sandbox_mode=\"$CODEX_SANDBOX\"")
  if [[ -n "$CODEX_MODEL" ]]; then
    CODEX_ARGS+=(-m "$CODEX_MODEL")
  fi
}

# run_codex runs codex with the prompt from stdin and prints the session id and the final answer.
run_codex() {
  local status=0 session
  "$CODEX_BIN" "$@" >"$work/events.jsonl" 2>"$work/stderr.txt" || status=$?
  session="$(sed -n 's/.*"type":"thread.started","thread_id":"\([^"]*\)".*/\1/p' "$work/events.jsonl" | head -n 1)"
  printf 'sessão: %s\n' "${session:-desconhecida}"
  printf 'código de saída: %s\n' "$status"
  if [[ -s "$work/last.txt" ]]; then
    printf '%s\n' '--- resposta final do Codex'
    cat "$work/last.txt"
  else
    printf '%s\n' '--- sem resposta final; últimos erros'
    grep '"type":"error"' "$work/events.jsonl" | tail -n 5 || true
    tail -n 5 "$work/stderr.txt" || true
  fi
  return "$status"
}

args=("$@")
if ((${#args[@]} == 0)) && [[ -n "${SSH_ORIGINAL_COMMAND:-}" ]]; then
  # Forced command in authorized_keys: the request arrives in SSH_ORIGINAL_COMMAND, with the
  # caller's backslash quoting, so read must not use -r.
  # shellcheck disable=SC2162
  read -a args <<<"$SSH_ORIGINAL_COMMAND"
  args=("${args[@]:1}")
fi

usage_text="uso: codex-server.sh exec | resume <sessão> | --check (o pedido vem pela entrada padrão)"
case "${args[0]:-}" in
  exec)
    ((${#args[@]} == 1)) || die "$usage_text"
    acquire_lock
    build_codex_args
    work="$(mktemp -d)"
    trap 'rm -rf "$work"' EXIT
    run_codex exec "${CODEX_ARGS[@]}" -C "$CODEX_REPO_DIR" -o "$work/last.txt" -
    ;;
  resume)
    ((${#args[@]} == 2)) && [[ "${args[1]}" =~ ^[A-Za-z0-9-]+$ ]] || die "$usage_text"
    acquire_lock
    build_codex_args
    work="$(mktemp -d)"
    trap 'rm -rf "$work"' EXIT
    run_codex exec resume "${CODEX_ARGS[@]}" -o "$work/last.txt" "${args[1]}" -
    ;;
  --check) print_check ;;
  *) die "$usage_text" ;;
esac
