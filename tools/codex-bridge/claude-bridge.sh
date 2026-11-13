#!/usr/bin/env bash
# Runs on the Claude machine: sends tasks to Codex on the other machine over SSH, mounts the
# project for reading and runs review commands there, with settings from a private .env.
# See docs/codex.md.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source-path=SCRIPTDIR
source "$script_dir/lib.sh"

load_env "${CODEX_BRIDGE_ENV:-$script_dir/.env}"
require_vars CODEX_HOST CODEX_SSH_USER CODEX_REMOTE_REPO
[[ "$CODEX_REMOTE_REPO" == /* ]] || die "CODEX_REMOTE_REPO precisa ser um caminho absoluto"

CODEX_SSH_PORT="${CODEX_SSH_PORT:-22}"
CODEX_SSH_KEY="${CODEX_SSH_KEY:-}"
CODEX_SSH_TOOLS_KEY="${CODEX_SSH_TOOLS_KEY:-$CODEX_SSH_KEY}"
CODEX_REMOTE_SERVER="${CODEX_REMOTE_SERVER:-$CODEX_REMOTE_REPO/tools/codex-bridge/codex-server.sh}"
CLAUDE_MOUNT_DIR="$(expand_home "${CLAUDE_MOUNT_DIR:-$HOME/radar-codex}")"
CLAUDE_MOUNT_READONLY="${CLAUDE_MOUNT_READONLY:-1}"
target="$CODEX_SSH_USER@$CODEX_HOST"

# build_ssh_opts fills SSH_OPTS with the connection options for the given private key.
build_ssh_opts() {
  local key="$1"
  SSH_OPTS=(-p "$CODEX_SSH_PORT" -o BatchMode=yes -o ServerAliveInterval=30)
  if [[ -n "$key" ]]; then
    SSH_OPTS+=(-i "$(expand_home "$key")" -o IdentitiesOnly=yes)
  fi
}

# remote_quote joins arguments into one shell-quoted string for the remote side.
remote_quote() {
  local quoted
  printf -v quoted '%q ' "$@"
  printf '%s' "${quoted% }"
}

# is_mounted reports whether the mount directory is an active mount point.
is_mounted() {
  if command -v mountpoint >/dev/null 2>&1; then
    mountpoint -q "$CLAUDE_MOUNT_DIR"
  else
    mount | grep -q " on $CLAUDE_MOUNT_DIR "
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

# cmd_task sends a new task to Codex and prints the session id and the final answer.
cmd_task() {
  (($# <= 1)) || die "uso: claude-bridge.sh task [arquivo-do-pedido]"
  open_prompt "${1:-}"
  build_ssh_opts "$CODEX_SSH_KEY"
  exec ssh -T "${SSH_OPTS[@]}" "$target" "$(remote_quote "$CODEX_REMOTE_SERVER" exec)"
}

# cmd_reply continues an existing Codex session with a follow-up prompt.
cmd_reply() {
  (($# >= 1 && $# <= 2)) || die "uso: claude-bridge.sh reply <sessão> [arquivo-do-pedido]"
  [[ "$1" =~ ^[A-Za-z0-9-]+$ ]] || die "identificador de sessão inválido: $1"
  open_prompt "${2:-}"
  build_ssh_opts "$CODEX_SSH_KEY"
  exec ssh -T "${SSH_OPTS[@]}" "$target" "$(remote_quote "$CODEX_REMOTE_SERVER" resume "$1")"
}

# cmd_run executes one command inside the project folder on the Codex machine.
cmd_run() {
  (($# > 0)) || die "uso: claude-bridge.sh run <comando> [argumentos]"
  build_ssh_opts "$CODEX_SSH_TOOLS_KEY"
  exec ssh -T "${SSH_OPTS[@]}" "$target" "cd $(remote_quote "$CODEX_REMOTE_REPO") && $(remote_quote "$@")"
}

# cmd_mount mounts the project folder of the Codex machine with sshfs, read-only by default.
cmd_mount() {
  command -v sshfs >/dev/null 2>&1 || die "sshfs não instalado nesta máquina"
  if is_mounted; then
    printf 'já montado em %s\n' "$CLAUDE_MOUNT_DIR"
    return
  fi
  mkdir -p "$CLAUDE_MOUNT_DIR"
  local opts="reconnect,ServerAliveInterval=15,BatchMode=yes,port=$CODEX_SSH_PORT"
  if [[ "$CLAUDE_MOUNT_READONLY" == "1" ]]; then
    opts="ro,$opts"
  fi
  if [[ -n "$CODEX_SSH_TOOLS_KEY" ]]; then
    opts="$opts,IdentityFile=$(expand_home "$CODEX_SSH_TOOLS_KEY"),IdentitiesOnly=yes"
  fi
  sshfs "$target:$CODEX_REMOTE_REPO" "$CLAUDE_MOUNT_DIR" -o "$opts"
  printf 'montado em %s\n' "$CLAUDE_MOUNT_DIR"
}

# cmd_unmount releases the sshfs mount on Linux and macOS.
cmd_unmount() {
  if ! is_mounted; then
    printf 'nada montado em %s\n' "$CLAUDE_MOUNT_DIR"
    return
  fi
  if command -v fusermount3 >/dev/null 2>&1; then
    fusermount3 -u "$CLAUDE_MOUNT_DIR"
  elif command -v fusermount >/dev/null 2>&1; then
    fusermount -u "$CLAUDE_MOUNT_DIR"
  else
    umount "$CLAUDE_MOUNT_DIR"
  fi
  printf 'desmontado: %s\n' "$CLAUDE_MOUNT_DIR"
}

# cmd_check verifies the local tools, the SSH access and the Codex setup on the other machine.
cmd_check() {
  local tool
  for tool in ssh sshfs; do
    if command -v "$tool" >/dev/null 2>&1; then
      printf '%s: ok\n' "$tool"
    else
      printf '%s: não encontrado nesta máquina\n' "$tool"
    fi
  done
  build_ssh_opts "$CODEX_SSH_KEY"
  printf 'conexão SSH com %s:\n' "$target"
  ssh -T "${SSH_OPTS[@]}" "$target" "$(remote_quote "$CODEX_REMOTE_SERVER" --check)"
  build_ssh_opts "$CODEX_SSH_TOOLS_KEY"
  if ssh -T "${SSH_OPTS[@]}" "$target" "cd $(remote_quote "$CODEX_REMOTE_REPO") && git rev-parse --is-inside-work-tree" >/dev/null 2>&1; then
    printf 'comandos de revisão (run): ok\n'
  else
    printf 'comandos de revisão (run): falhou, confira CODEX_SSH_TOOLS_KEY\n'
  fi
  if is_mounted; then
    printf 'montagem: ativa em %s\n' "$CLAUDE_MOUNT_DIR"
  else
    printf 'montagem: inativa, rode claude-bridge.sh mount\n'
  fi
}

# usage prints the available subcommands.
usage() {
  cat <<'EOF'
uso: claude-bridge.sh <comando>

  task [arquivo]           envia uma tarefa nova ao Codex (pedido no arquivo ou na entrada padrão)
  reply <sessão> [arquivo] continua a sessão do Codex com um novo pedido
  run <comando>            roda um comando na pasta do projeto, como: run git diff
  mount                    monta a pasta do projeto com sshfs (somente leitura por padrão)
  unmount                  desmonta a pasta
  check                    confere ferramentas locais, SSH e o Codex na outra máquina
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
