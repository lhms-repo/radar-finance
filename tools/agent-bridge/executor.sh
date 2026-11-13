#!/usr/bin/env bash
# Runs on the executor machine: runs one coding agent task non-interactively inside the project, with
# settings from a private .env. planner.sh calls it over SSH and sends the prompt on stdin.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source-path=SCRIPTDIR
source "$script_dir/lib.sh"

load_env "${BRIDGE_ENV:-$script_dir/.env}"

REPO_DIR="$(expand_home "${REPO_DIR:-$(cd "$script_dir/../.." && pwd)}")"
AGENT_BIN="$(expand_home "${AGENT_BIN:-codex}")"
AGENT_SANDBOX="${AGENT_SANDBOX:-workspace-write}"
AGENT_MODEL="${AGENT_MODEL:-}"
if [[ -n "${AGENT_PATH_EXTRA:-}" ]]; then
  export PATH="$AGENT_PATH_EXTRA:$PATH"
fi
if [[ -n "${AGENT_HOME_DIR:-}" ]]; then
  CODEX_HOME="$(expand_home "$AGENT_HOME_DIR")"
  export CODEX_HOME
fi

[[ -d "$REPO_DIR" ]] || die "REPO_DIR não existe: $REPO_DIR"
[[ -f "$REPO_DIR/AGENTS.md" ]] || die "AGENTS.md não encontrado em $REPO_DIR"
command -v "$AGENT_BIN" >/dev/null 2>&1 || die "agente não encontrado: $AGENT_BIN. Ajuste AGENT_BIN ou AGENT_PATH_EXTRA."
case "$AGENT_SANDBOX" in
  read-only | workspace-write) ;;
  *) die "AGENT_SANDBOX aceita só read-only ou workspace-write" ;;
esac
cd "$REPO_DIR"

# print_check reports the resolved settings and tool versions, for a manual test over SSH.
print_check() {
  printf 'pasta do projeto: %s\n' "$REPO_DIR"
  printf 'agente: %s (%s)\n' "$(command -v "$AGENT_BIN")" "$("$AGENT_BIN" --version 2>/dev/null | head -n 1)"
  printf 'login do agente: %s\n' "$("$AGENT_BIN" login status 2>&1 | tail -n 1)"
  printf 'CODEX_HOME: %s\n' "${CODEX_HOME:-padrão (~/.codex)}"
  printf 'sandbox: %s\n' "$AGENT_SANDBOX"
  printf 'modelo: %s\n' "${AGENT_MODEL:-padrão da configuração do agente}"
  local tool
  for tool in go pnpm node make git flock; do
    if command -v "$tool" >/dev/null 2>&1; then
      printf '%s: ok\n' "$tool"
    else
      printf '%s: não encontrado no PATH, ajuste AGENT_PATH_EXTRA\n' "$tool"
    fi
  done
}

# acquire_lock makes sure only one agent task runs on this machine at a time.
acquire_lock() {
  if ! command -v flock >/dev/null 2>&1; then
    return 0
  fi
  exec 9>"${XDG_RUNTIME_DIR:-/tmp}/radar-agent-bridge.lock"
  flock -n 9 || die "já existe uma tarefa do agente em andamento nesta máquina"
}

# build_agent_args fills AGENT_ARGS with the options shared by new and resumed tasks.
build_agent_args() {
  AGENT_ARGS=(--json -c "sandbox_mode=\"$AGENT_SANDBOX\"")
  if [[ -n "$AGENT_MODEL" ]]; then
    AGENT_ARGS+=(-m "$AGENT_MODEL")
  fi
}

# run_agent runs the agent with the prompt from stdin and prints the session id and the final answer.
run_agent() {
  local status=0 session
  "$AGENT_BIN" "$@" >"$work/events.jsonl" 2>"$work/stderr.txt" || status=$?
  session="$(sed -n 's/.*"type":"thread.started","thread_id":"\([^"]*\)".*/\1/p' "$work/events.jsonl" | head -n 1)"
  printf 'sessão: %s\n' "${session:-desconhecida}"
  printf 'código de saída: %s\n' "$status"
  if [[ -s "$work/last.txt" ]]; then
    printf '%s\n' '--- resposta final do agente'
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

usage_text="uso: executor.sh exec | resume <sessão> | --check (o pedido vem pela entrada padrão)"
case "${args[0]:-}" in
  exec)
    ((${#args[@]} == 1)) || die "$usage_text"
    acquire_lock
    build_agent_args
    work="$(mktemp -d)"
    trap 'rm -rf "$work"' EXIT
    run_agent exec "${AGENT_ARGS[@]}" -C "$REPO_DIR" -o "$work/last.txt" -
    ;;
  resume)
    ((${#args[@]} == 2)) && [[ "${args[1]}" =~ ^[A-Za-z0-9-]+$ ]] || die "$usage_text"
    acquire_lock
    build_agent_args
    work="$(mktemp -d)"
    trap 'rm -rf "$work"' EXIT
    run_agent exec resume "${AGENT_ARGS[@]}" -o "$work/last.txt" "${args[1]}" -
    ;;
  --check) print_check ;;
  *) die "$usage_text" ;;
esac
