#!/usr/bin/env bash
# Runs on the planner machine: sends tasks to the coding agent on the executor machine over SSH, mounts the
# project for reading and runs review commands there, with settings from a private .env.
# See docs/agentes.md.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source-path=SCRIPTDIR
source "$script_dir/lib.sh"

load_env "${BRIDGE_ENV:-$script_dir/.env}"
require_vars EXECUTOR_HOST EXECUTOR_SSH_USER EXECUTOR_REPO_DIR
[[ "$EXECUTOR_REPO_DIR" == /* ]] || die "EXECUTOR_REPO_DIR precisa ser um caminho absoluto"

EXECUTOR_SSH_PORT="${EXECUTOR_SSH_PORT:-22}"
EXECUTOR_SSH_KEY="${EXECUTOR_SSH_KEY:-}"
EXECUTOR_SSH_TOOLS_KEY="${EXECUTOR_SSH_TOOLS_KEY:-$EXECUTOR_SSH_KEY}"
EXECUTOR_SCRIPT="${EXECUTOR_SCRIPT:-$EXECUTOR_REPO_DIR/tools/agent-bridge/executor.sh}"
PLANNER_MOUNT_DIR="$(expand_home "${PLANNER_MOUNT_DIR:-$HOME/radar-remote}")"
PLANNER_MOUNT_READONLY="${PLANNER_MOUNT_READONLY:-1}"
target="$EXECUTOR_SSH_USER@$EXECUTOR_HOST"

# build_ssh_opts fills SSH_OPTS with the connection options for the given private key.
build_ssh_opts() {
  local key="$1"
  SSH_OPTS=(-p "$EXECUTOR_SSH_PORT" -o BatchMode=yes -o ServerAliveInterval=30)
  if [[ -n "$key" ]]; then
    SSH_OPTS+=(-i "$(expand_home "$key")" -o IdentitiesOnly=yes)
  fi
}

# is_mounted reports whether the mount directory is an active mount point.
is_mounted() {
  if command -v mountpoint >/dev/null 2>&1; then
    mountpoint -q "$PLANNER_MOUNT_DIR"
  else
    mount | grep -q " on $PLANNER_MOUNT_DIR "
  fi
}

# open_prompt points stdin at the prompt file, or keeps a piped stdin when no file is given.
open_prompt() {
  local file="${1:-}"
  if [[ -n "$file" ]]; then
    [[ -f "$file" ]] || die "arquivo de pedido não encontrado: $file"
    exec <"$file"
  elif [[ -t 0 ]]; then
    die "passe o arquivo do pedido ou envie o pedido pela entrada padrão"
  fi
}

# cmd_task sends a new task to the agent and prints the session id and the final answer.
cmd_task() {
  (($# <= 1)) || die "uso: planner.sh task [arquivo-do-pedido]"
  open_prompt "${1:-}"
  build_ssh_opts "$EXECUTOR_SSH_KEY"
  exec ssh -T "${SSH_OPTS[@]}" "$target" "$(remote_quote "$EXECUTOR_SCRIPT" exec)"
}

# cmd_reply continues an existing agent session with a follow-up prompt.
cmd_reply() {
  (($# >= 1 && $# <= 2)) || die "uso: planner.sh reply <sessão> [arquivo-do-pedido]"
  [[ "$1" =~ ^[A-Za-z0-9-]+$ ]] || die "identificador de sessão inválido: $1"
  open_prompt "${2:-}"
  build_ssh_opts "$EXECUTOR_SSH_KEY"
  exec ssh -T "${SSH_OPTS[@]}" "$target" "$(remote_quote "$EXECUTOR_SCRIPT" resume "$1")"
}

# cmd_run executes one command inside the project folder on the executor machine.
cmd_run() {
  (($# > 0)) || die "uso: planner.sh run <comando> [argumentos]"
  build_ssh_opts "$EXECUTOR_SSH_TOOLS_KEY"
  exec ssh -T "${SSH_OPTS[@]}" "$target" "cd $(remote_quote "$EXECUTOR_REPO_DIR") && $(remote_quote "$@")"
}

# cmd_mount mounts the project folder of the executor machine with sshfs, read-only by default.
cmd_mount() {
  command -v sshfs >/dev/null 2>&1 || die "sshfs não instalado nesta máquina"
  if is_mounted; then
    printf 'já montado em %s\n' "$PLANNER_MOUNT_DIR"
    return
  fi
  mkdir -p "$PLANNER_MOUNT_DIR"
  local opts="reconnect,ServerAliveInterval=15,BatchMode=yes,port=$EXECUTOR_SSH_PORT"
  if [[ "$PLANNER_MOUNT_READONLY" == "1" ]]; then
    opts="ro,$opts"
  fi
  if [[ -n "$EXECUTOR_SSH_TOOLS_KEY" ]]; then
    opts="$opts,IdentityFile=$(expand_home "$EXECUTOR_SSH_TOOLS_KEY"),IdentitiesOnly=yes"
  fi
  sshfs "$target:$EXECUTOR_REPO_DIR" "$PLANNER_MOUNT_DIR" -o "$opts"
  printf 'montado em %s\n' "$PLANNER_MOUNT_DIR"
}

# cmd_unmount releases the sshfs mount on Linux and macOS.
cmd_unmount() {
  if ! is_mounted; then
    printf 'nada montado em %s\n' "$PLANNER_MOUNT_DIR"
    return
  fi
  if command -v fusermount3 >/dev/null 2>&1; then
    fusermount3 -u "$PLANNER_MOUNT_DIR"
  elif command -v fusermount >/dev/null 2>&1; then
    fusermount -u "$PLANNER_MOUNT_DIR"
  else
    umount "$PLANNER_MOUNT_DIR"
  fi
  printf 'desmontado: %s\n' "$PLANNER_MOUNT_DIR"
}

# cmd_check verifies the local tools, the SSH access and the agent setup on the executor machine.
cmd_check() {
  local tool
  for tool in ssh sshfs; do
    if command -v "$tool" >/dev/null 2>&1; then
      printf '%s: ok\n' "$tool"
    else
      printf '%s: não encontrado nesta máquina\n' "$tool"
    fi
  done
  build_ssh_opts "$EXECUTOR_SSH_KEY"
  printf 'conexão SSH com %s:\n' "$target"
  ssh -T "${SSH_OPTS[@]}" "$target" "$(remote_quote "$EXECUTOR_SCRIPT" --check)"
  build_ssh_opts "$EXECUTOR_SSH_TOOLS_KEY"
  if ! ssh -T "${SSH_OPTS[@]}" "$target" "cd $(remote_quote "$EXECUTOR_REPO_DIR")" >/dev/null 2>&1; then
    printf 'comandos de revisão (run): sem acesso, confira EXECUTOR_SSH_TOOLS_KEY e EXECUTOR_REPO_DIR\n'
  elif ! ssh -T "${SSH_OPTS[@]}" "$target" "cd $(remote_quote "$EXECUTOR_REPO_DIR") && git rev-parse --is-inside-work-tree" >/dev/null 2>&1; then
    printf 'comandos de revisão (run): ok, mas a pasta não é um repositório git, então git diff não vai funcionar\n'
  else
    printf 'comandos de revisão (run): ok\n'
  fi
  if is_mounted; then
    printf 'montagem: ativa em %s\n' "$PLANNER_MOUNT_DIR"
  else
    printf 'montagem: inativa, rode planner.sh mount\n'
  fi
}

# usage prints the available subcommands.
usage() {
  cat <<'EOF'
uso: planner.sh <comando>

  task [arquivo]           envia uma tarefa nova ao agente (pedido no arquivo ou na entrada padrão)
  reply <sessão> [arquivo] continua a sessão do agente com um novo pedido
  run <comando>            roda um comando na pasta do projeto, como: run git diff
  mount                    monta a pasta do projeto com sshfs (somente leitura por padrão)
  unmount                  desmonta a pasta
  check                    confere ferramentas locais, SSH e o agente na máquina executora
EOF
}

case "${1:-}" in
  task) shift; cmd_task "$@" ;;
  reply) shift; cmd_reply "$@" ;;
  run) shift; cmd_run "$@" ;;
  mount) cmd_mount ;;
  unmount) cmd_unmount ;;
  check) cmd_check ;;
  *) usage; exit 1 ;;
esac
