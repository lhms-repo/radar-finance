#!/usr/bin/env bash
# Prepares either machine of the agent bridge: the executor (SSH server, agent paths, .env) or
# the planner (SSH key, copy of the scripts, .env). Safe to run again: it only fills what is missing.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source-path=SCRIPTDIR
source "$script_dir/lib.sh"

bridge_files=(lib.sh planner.sh executor.sh setup.sh .env.example)

# info prints a progress line.
info() {
  printf '==> %s\n' "$*"
}

# warn prints a warning that does not stop the setup.
warn() {
  printf 'aviso: %s\n' "$*" >&2
}

# as_root runs a command with sudo unless the script already runs as root.
as_root() {
  if ((EUID == 0)); then
    "$@"
  else
    sudo "$@"
  fi
}

# has_systemd reports whether systemd manages services on this machine.
has_systemd() {
  command -v systemctl >/dev/null 2>&1 && [[ -d /run/systemd/system ]]
}

# ensure_env_file creates the .env from the example with mode 600 when it does not exist yet.
ensure_env_file() {
  local file="$1" example="$2"
  if [[ ! -f "$file" ]]; then
    (umask 077 && cp "$example" "$file")
    info "criado $file"
  fi
  chmod 600 "$file"
}

# set_env_value writes KEY=VALUE in the .env; with force 0 it keeps a value the user already set.
set_env_value() {
  local file="$1" key="$2" value="$3" force="$4" tmp
  tmp="$(mktemp "$file.XXXXXX")"
  chmod 600 "$tmp"
  awk -v key="$key" -v value="$value" -v force="$force" '
    index($0, key "=") == 1 {
      current = substr($0, length(key) + 2)
      if (current == "" || force == "1") { print key "=" value } else { print }
      found = 1
      next
    }
    { print }
    END { if (!found) print key "=" value }
  ' "$file" >"$tmp"
  mv "$tmp" "$file"
}

# print_windows_steps shows the PowerShell commands that WSL cannot run on its own.
print_windows_steps() {
  cat <<'EOF'

Passos no Windows (PowerShell como administrador). Se esse PC for a produção, o
wsl --shutdown derruba o Docker e o Radar por alguns instantes: faça fora de horário.

  # Só se %UserProfile%\.wslconfig ainda não tiver uma seção [wsl2]; se tiver,
  # acrescente nela a linha networkingMode=mirrored.
  Add-Content -Path "$env:USERPROFILE\.wslconfig" -Value "`n[wsl2]`nnetworkingMode=mirrored"
  New-NetFirewallRule -DisplayName "Radar SSH executor" -Direction Inbound -Protocol TCP -LocalPort 22 -Profile Private -Action Allow
  wsl --shutdown

Depois, abra o WSL de novo e rode este setup mais uma vez para conferir.
EOF
}

# lan_prefix prints the /24 network of this machine's first IPv4 address, such as 192.168.0.0/24.
lan_prefix() {
  hostname -I 2>/dev/null | awk '{ split($1, o, "."); if (o[4] != "") print o[1] "." o[2] "." o[3] ".0/24" }'
}

# check_firewall opens SSH to the local network when ufw is active and has no rule for port 22.
check_firewall() {
  command -v ufw >/dev/null 2>&1 || return 0
  local status prefix
  status="$(as_root ufw status 2>/dev/null || true)"
  [[ "$status" == *"Status: active"* ]] || return 0
  if grep -Eq '^(22|22/tcp|OpenSSH)[[:space:]]' <<<"$status"; then
    info "firewall: a porta 22 já está liberada"
    return 0
  fi
  prefix="$(lan_prefix)"
  if [[ -z "$prefix" ]]; then
    warn "firewall ativo sem regra para a porta 22; libere com: sudo ufw allow from <sua-rede>/24 to any port 22 proto tcp"
    return 0
  fi
  info "firewall: liberando a porta 22 só para a rede local $prefix"
  as_root ufw allow from "$prefix" to any port 22 proto tcp
}

# setup_executor prepares the machine that runs the coding agent inside the project.
setup_executor() {
  (($# == 0)) || die "uso: setup.sh executor"
  local repo env_file agent tool dir extra="" missing=() ip
  repo="$(cd "$script_dir/../.." && pwd)"
  env_file="$script_dir/.env"
  [[ -f "$repo/AGENTS.md" ]] || die "rode este script de dentro do repositório (AGENTS.md não encontrado em $repo)"
  if ((EUID == 0)); then
    warn "rodando como root; o planejador vai entrar com o usuário que você usar aqui"
  fi

  info "servidor SSH"
  if [[ ! -x /usr/sbin/sshd ]]; then
    command -v apt-get >/dev/null 2>&1 || die "instale o servidor OpenSSH e rode de novo"
    as_root apt-get update -qq
    as_root apt-get install -y -qq openssh-server
  fi
  if has_systemd; then
    as_root systemctl enable --now ssh
  else
    warn "systemd não está ativo; inicie o SSH com: sudo service ssh start"
  fi

  info "agente e ferramentas"
  agent="$(command -v codex || true)"
  [[ -n "$agent" ]] || die "agente não encontrado no PATH; instale o Codex CLI e rode de novo"
  if ! "$agent" login status >/dev/null 2>&1; then
    warn "agente sem login; rode: codex login"
  fi
  for tool in codex go pnpm node; do
    if command -v "$tool" >/dev/null 2>&1; then
      dir="$(dirname "$(command -v "$tool")")"
      case "$dir" in
        /usr/bin | /bin | /usr/local/bin) ;;
        *) [[ ":$extra:" == *":$dir:"* ]] || extra="${extra:+$extra:}$dir" ;;
      esac
    else
      missing+=("$tool")
    fi
  done
  for tool in make git flock; do
    command -v "$tool" >/dev/null 2>&1 || missing+=("$tool")
  done
  if ((${#missing[@]} > 0)); then
    warn "não encontrados: ${missing[*]}. Instale nesta máquina (Go e pnpm nas versões do .devcontainer) e rode de novo."
  fi

  info "arquivo $env_file"
  ensure_env_file "$env_file" "$script_dir/.env.example"
  set_env_value "$env_file" AGENT_BIN "$agent" 0
  if [[ -n "$extra" ]]; then
    set_env_value "$env_file" AGENT_PATH_EXTRA "$extra" 0
  fi

  check_firewall

  if grep -qi microsoft /proc/version 2>/dev/null; then
    info "rede do WSL"
    if command -v wslinfo >/dev/null 2>&1 && [[ "$(wslinfo --networking-mode 2>/dev/null || true)" == "mirrored" ]]; then
      info "rede espelhada ativa; confira só a regra de firewall da porta 22 no Windows"
    else
      print_windows_steps
    fi
  fi

  info "conferência"
  "$script_dir/executor.sh" --check

  ip="$(hostname -I 2>/dev/null | awk '{print $1}')"
  cat <<EOF

Próximo passo, na máquina do planejador:

  ssh $(id -un)@${ip:-<ip-desta-maquina>} 'tar --exclude=agent-bridge/.env -C $(remote_quote "$repo/tools") -cf - agent-bridge' | tar -C ~ -xf -
  ~/agent-bridge/setup.sh planner $(id -un)@${ip:-<ip-desta-maquina>} --repo $(remote_quote "$repo")
EOF
}

# setup_planner prepares the machine that plans and reviews: key, scripts copy and .env.
setup_planner() {
  local target="" user host repo="" port=22 key="$HOME/.ssh/radar_executor" dir="$script_dir"
  while (($# > 0)); do
    case "$1" in
      --repo) repo="${2:?}"; shift 2 ;;
      --port) port="${2:?}"; shift 2 ;;
      --key) key="$(expand_home "${2:?}")"; shift 2 ;;
      --dir) dir="$(expand_home "${2:?}")"; shift 2 ;;
      -*) die "opção desconhecida: $1" ;;
      *) [[ -z "$target" ]] || die "destino repetido: $1"; target="$1"; shift ;;
    esac
  done
  [[ "$target" == *@* ]] || die "uso: setup.sh planner usuario@host [--repo caminho] [--port 22] [--key arquivo] [--dir pasta]"
  user="${target%@*}"
  host="${target#*@}"
  repo="${repo:-/home/$user/radar-finance}"
  [[ "$repo" == /* ]] || die "--repo precisa ser um caminho absoluto"
  [[ "$port" =~ ^[0-9]+$ ]] || die "--port precisa ser um número"

  local tool
  for tool in ssh ssh-keygen ssh-copy-id tar; do
    command -v "$tool" >/dev/null 2>&1 || die "$tool não encontrado nesta máquina"
  done

  info "sshfs"
  if ! command -v sshfs >/dev/null 2>&1; then
    if command -v apt-get >/dev/null 2>&1; then
      as_root apt-get install -y -qq sshfs || warn "não consegui instalar o sshfs; instale à mão para usar o mount"
    else
      warn "instale o sshfs (no macOS, o macFUSE e o sshfs) para usar o mount"
    fi
  fi

  info "chave SSH $key"
  if [[ ! -f "$key" ]]; then
    mkdir -p "$(dirname "$key")"
    chmod 700 "$(dirname "$key")"
    ssh-keygen -t ed25519 -f "$key" -C radar-executor
  fi
  local ssh_opts=(-p "$port" -i "$key" -o IdentitiesOnly=yes)
  if ! ssh "${ssh_opts[@]}" -o BatchMode=yes "$target" true 2>/dev/null; then
    info "autorizando a chave em $target (pede a senha de $user uma vez)"
    ssh-copy-id -p "$port" -i "$key.pub" "$target"
  fi
  ssh "${ssh_opts[@]}" -o BatchMode=yes "$target" true || die "a chave não foi aceita por $target"

  info "scripts da ponte em $dir"
  ssh "${ssh_opts[@]}" -o BatchMode=yes "$target" "test -x $(remote_quote "$repo/tools/agent-bridge/executor.sh")" ||
    die "executor.sh não encontrado em $repo; rode setup.sh executor na máquina executora antes"
  local tmp
  tmp="$(mktemp -d)"
  ssh "${ssh_opts[@]}" -o BatchMode=yes "$target" \
    "tar -C $(remote_quote "$repo/tools/agent-bridge") -cf - $(remote_quote "${bridge_files[@]}")" | tar -C "$tmp" -xf -
  mkdir -p "$dir"
  local file
  for file in "${bridge_files[@]}"; do
    mv -f "$tmp/$file" "$dir/$file"
  done
  rm -rf "$tmp"

  info "arquivo $dir/.env"
  ensure_env_file "$dir/.env" "$dir/.env.example"
  set_env_value "$dir/.env" EXECUTOR_HOST "$host" 1
  set_env_value "$dir/.env" EXECUTOR_SSH_USER "$user" 1
  set_env_value "$dir/.env" EXECUTOR_SSH_PORT "$port" 1
  set_env_value "$dir/.env" EXECUTOR_SSH_KEY "$key" 1
  set_env_value "$dir/.env" EXECUTOR_REPO_DIR "$repo" 1

  info "conferência"
  "$dir/planner.sh" check

  cat <<EOF

Próximos passos:

  $dir/planner.sh mount
  echo "Responda apenas: ok. Não altere nenhum arquivo." | $dir/planner.sh task

Quando a tarefa de teste responder, desligue o login por senha na máquina executora:

  $repo/tools/agent-bridge/setup.sh lock-password
EOF
}

# lock_password turns off SSH password login once the planner key is authorized.
lock_password() {
  (($# == 0)) || die "uso: setup.sh lock-password"
  [[ -s "$HOME/.ssh/authorized_keys" ]] || die "nenhuma chave autorizada em ~/.ssh/authorized_keys; rode setup.sh planner antes"
  grep -q '^Include /etc/ssh/sshd_config.d/' /etc/ssh/sshd_config ||
    die "/etc/ssh/sshd_config não inclui sshd_config.d; ajuste PasswordAuthentication à mão"
  printf 'PasswordAuthentication no\nKbdInteractiveAuthentication no\n' |
    as_root tee /etc/ssh/sshd_config.d/10-radar.conf >/dev/null
  as_root /usr/sbin/sshd -t
  if has_systemd; then
    as_root systemctl restart ssh
  else
    as_root service ssh restart
  fi
  info "login por senha desligado. Antes de fechar este terminal, confira que a chave entra: planner.sh check"
}

# usage prints the available modes.
usage() {
  cat <<'EOF'
uso: setup.sh <modo>

  executor                       prepara esta máquina como executora (rode de dentro do repositório)
  planner usuario@host [opções]  prepara esta máquina como planejadora
      --repo caminho             projeto na máquina executora (padrão: /home/usuario/radar-finance)
      --port 22                  porta SSH da máquina executora
      --key arquivo              chave SSH (padrão: ~/.ssh/radar_executor)
      --dir pasta                onde ficam os scripts (padrão: a pasta deste script)
  lock-password                  desliga o login por senha na máquina executora
EOF
}

# main dispatches to the chosen mode.
main() {
  local mode="${1:-}"
  shift || true
  case "$mode" in
    executor) setup_executor "$@" ;;
    planner) setup_planner "$@" ;;
    lock-password) lock_password "$@" ;;
    *) usage; exit 1 ;;
  esac
}

main "$@"; exit
