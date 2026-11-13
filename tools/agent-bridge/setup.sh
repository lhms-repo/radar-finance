#!/usr/bin/env bash
# Installs and removes the agent bridge. On the planner, apply and remove read a topology file
# and reach the executor over SSH; executor and planner prepare only the local machine.
# Safe to run again: it only fills what is missing, and records what it changed so remove can
# undo exactly that. It never installs or removes the coding agent itself.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source-path=SCRIPTDIR
source "$script_dir/lib.sh"

bridge_files=(lib.sh planner.sh executor.sh setup.sh .env.example topology.env.example)
apply_agent_keys=(AGENT_SANDBOX AGENT_MODEL AGENT_HOME_DIR)
sshd_dropin=/etc/ssh/sshd_config.d/10-radar.conf

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

# restart_ssh restarts the SSH server with systemd or the service command.
restart_ssh() {
  if has_systemd; then
    as_root systemctl restart ssh
  else
    as_root service ssh restart
  fi
}

# ensure_env_file creates a file from its example with mode 600 when it does not exist yet.
ensure_env_file() {
  local file="$1" example="$2"
  if [[ ! -f "$file" ]]; then
    (umask 077 && cp "$example" "$file")
    info "criado $file"
  fi
  chmod 600 "$file"
}

# set_env_value writes KEY=VALUE in a file; with force 0 it keeps a value the user already set.
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

# state_set records in a .state file something this setup changed, so remove can undo it.
state_set() {
  local file="$1"
  if [[ ! -f "$file" ]]; then
    (umask 077 && : >"$file")
  fi
  set_env_value "$file" "$2" "$3" 1
}

# state_get prints a value recorded in a .state file, or nothing.
state_get() {
  local file="$1" key="$2"
  [[ -f "$file" ]] || return 0
  awk -v key="$key" 'index($0, key "=") == 1 { print substr($0, length(key) + 2) }' "$file"
}

# env_get prints a value from a .env file without loading it, or nothing.
env_get() {
  state_get "$@"
}

# lan_prefix prints the /24 network of this machine's first IPv4 address, such as 192.168.0.0/24.
lan_prefix() {
  hostname -I 2>/dev/null | awk '{ split($1, o, "."); if (o[4] != "") print o[1] "." o[2] "." o[3] ".0/24" }'
}

# check_firewall opens SSH to the local network when ufw is active and has no rule for port 22.
check_firewall() {
  local state="$1" status prefix
  command -v ufw >/dev/null 2>&1 || return 0
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
  state_set "$state" STATE_UFW_RULE "$prefix"
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

# interactive_path prints the PATH of the user's interactive shell, which non-interactive SSH sessions lack.
interactive_path() {
  bash -lic 'printf "\n__BRIDGE_PATH__%s\n" "$PATH"' </dev/null 2>/dev/null | sed -n 's/^__BRIDGE_PATH__//p' | tail -n 1 || true
}

# setup_executor prepares the machine that runs the coding agent inside the project.
setup_executor() {
  (($# == 0)) || die "uso: setup.sh executor"
  local repo env_file state agent tool dir extra="" missing=() ip
  repo="$(cd "$script_dir/../.." && pwd)"
  env_file="$script_dir/.env"
  state="$script_dir/.state"
  [[ -f "$repo/AGENTS.md" ]] || die "rode este script de dentro do repositório (AGENTS.md não encontrado em $repo)"
  if ((EUID == 0)); then
    warn "rodando como root; o planejador vai entrar com o usuário que você usar aqui"
  fi

  info "servidor SSH"
  if [[ ! -x /usr/sbin/sshd ]]; then
    command -v apt-get >/dev/null 2>&1 || die "instale o servidor OpenSSH e rode de novo"
    as_root apt-get update -qq
    as_root apt-get install -y -qq openssh-server
    state_set "$state" STATE_INSTALLED_OPENSSH 1
  fi
  if has_systemd; then
    as_root systemctl enable --now ssh
  else
    warn "systemd não está ativo; inicie o SSH com: sudo service ssh start"
  fi
  check_firewall "$state"

  info "agente e ferramentas"
  local user_path
  user_path="$(interactive_path)"
  if [[ -n "$user_path" ]]; then
    PATH="$user_path:$PATH"
  fi
  agent="$(expand_home "$(env_get "$env_file" AGENT_BIN)")"
  if [[ -z "$agent" || ! -x "$agent" ]]; then
    agent="$(command -v codex || true)"
  fi
  [[ -n "$agent" ]] || die "agente não encontrado no PATH; instale o Codex CLI e rode de novo"
  if ! "$agent" login status >/dev/null 2>&1; then
    warn "agente sem login; rode nesta máquina: codex login"
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

  if [[ -n "${BRIDGE_APPLY:-}" ]]; then
    return 0
  fi
  ip="$(hostname -I 2>/dev/null | awk '{print $1}')"
  cat <<EOF

Próximo passo, na máquina do planejador:

  ssh $(id -un)@${ip:-<ip-desta-maquina>} 'tar --exclude=agent-bridge/.env --exclude=agent-bridge/.state -C $(remote_quote "$repo/tools") -cf - agent-bridge' | tar -C ~ -xf -
  ~/agent-bridge/setup.sh planner $(id -un)@${ip:-<ip-desta-maquina>} --repo $(remote_quote "$repo")
EOF
}

# require_planner_tools fails when a client tool the planner needs is missing.
require_planner_tools() {
  local tool
  for tool in ssh ssh-keygen ssh-copy-id tar timeout; do
    command -v "$tool" >/dev/null 2>&1 || die "$tool não encontrado nesta máquina"
  done
}

# install_sshfs installs sshfs with apt when it is missing, without stopping the setup on failure.
install_sshfs() {
  local state="$1"
  info "sshfs"
  if command -v sshfs >/dev/null 2>&1; then
    return 0
  fi
  if command -v apt-get >/dev/null 2>&1 && as_root apt-get install -y -qq sshfs; then
    state_set "$state" STATE_INSTALLED_SSHFS 1
  else
    warn "não consegui instalar o sshfs; instale à mão para usar o mount"
  fi
}

# authorize_key creates the planner key when needed and authorizes it on the executor.
authorize_key() {
  local target="$1" port="$2" key="$3" state="$4"
  info "chave SSH $key"
  if [[ ! -f "$key" ]]; then
    mkdir -p "$(dirname "$key")"
    chmod 700 "$(dirname "$key")"
    ssh-keygen -t ed25519 -f "$key" -C radar-executor
    state_set "$state" STATE_CREATED_KEY 1
  fi
  SSH_OPTS=(-p "$port" -i "$key" -o IdentitiesOnly=yes)
  if ! ssh "${SSH_OPTS[@]}" -o BatchMode=yes "$target" true 2>/dev/null; then
    info "autorizando a chave em $target (pede a senha de ${target%@*} uma vez)"
    ssh-copy-id -p "$port" -i "$key.pub" "$target"
  fi
  ssh "${SSH_OPTS[@]}" -o BatchMode=yes "$target" true || die "a chave não foi aceita por $target"
}

# require_remote_repo fails when the executor project lacks the bridge or AGENTS.md.
require_remote_repo() {
  local target="$1" repo="$2"
  ssh "${SSH_OPTS[@]}" -o BatchMode=yes "$target" \
    "test -x $(remote_quote "$repo/tools/agent-bridge/setup.sh") && test -f $(remote_quote "$repo/AGENTS.md")" ||
    die "a ponte não está em $repo na executora; atualize o repositório lá (git pull) e rode de novo"
}

# fetch_scripts copies the bridge scripts from the executor project into the planner folder.
fetch_scripts() {
  local target="$1" repo="$2" dir="$3" tmp file
  info "scripts da ponte em $dir"
  tmp="$(mktemp -d)"
  ssh "${SSH_OPTS[@]}" -o BatchMode=yes "$target" \
    "tar -C $(remote_quote "$repo/tools/agent-bridge") -cf - $(remote_quote "${bridge_files[@]}")" | tar -C "$tmp" -xf -
  mkdir -p "$dir"
  for file in "${bridge_files[@]}"; do
    mv -f "$tmp/$file" "$dir/$file"
  done
  rm -rf "$tmp"
}

# write_planner_env fills the planner .env with how to reach the executor.
write_planner_env() {
  local dir="$1" host="$2" user="$3" port="$4" key="$5" repo="$6"
  info "arquivo $dir/.env"
  ensure_env_file "$dir/.env" "$dir/.env.example"
  set_env_value "$dir/.env" EXECUTOR_HOST "$host" 1
  set_env_value "$dir/.env" EXECUTOR_SSH_USER "$user" 1
  set_env_value "$dir/.env" EXECUTOR_SSH_PORT "$port" 1
  set_env_value "$dir/.env" EXECUTOR_SSH_KEY "$key" 1
  set_env_value "$dir/.env" EXECUTOR_REPO_DIR "$repo" 1
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
  mkdir -p "$dir"

  require_planner_tools
  install_sshfs "$dir/.state"
  authorize_key "$target" "$port" "$key" "$dir/.state"
  require_remote_repo "$target" "$repo"
  fetch_scripts "$target" "$repo" "$dir"
  write_planner_env "$dir" "$host" "$user" "$port" "$key" "$repo"

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

# load_topology reads the topology file and fills the TOPO_* variables used by apply and remove.
load_topology() {
  local file="$1"
  load_env "$file"
  require_vars EXECUTOR_HOST EXECUTOR_SSH_USER EXECUTOR_REPO_DIR
  TOPO_HOST="$EXECUTOR_HOST"
  TOPO_USER="$EXECUTOR_SSH_USER"
  TOPO_REPO="$EXECUTOR_REPO_DIR"
  TOPO_PORT="${EXECUTOR_SSH_PORT:-22}"
  TOPO_KEY="$(expand_home "${EXECUTOR_SSH_KEY:-$HOME/.ssh/radar_executor}")"
  TOPO_DIR="$(expand_home "${PLANNER_DIR:-$script_dir}")"
  TOPO_TARGET="$TOPO_USER@$TOPO_HOST"
  [[ "$TOPO_REPO" == /* ]] || die "EXECUTOR_REPO_DIR precisa ser um caminho absoluto"
  [[ "$TOPO_PORT" =~ ^[0-9]+$ ]] || die "EXECUTOR_SSH_PORT precisa ser um número"
  TTY_OPTS=()
  if [[ -t 0 ]]; then
    TTY_OPTS=(-t)
  fi
}

# setup_apply prepares both machines from a topology file, reaching the executor over SSH.
setup_apply() {
  local file="" run_test=0 lock=0
  while (($# > 0)); do
    case "$1" in
      --test) run_test=1; shift ;;
      --lock-password) lock=1; shift ;;
      -*) die "opção desconhecida: $1" ;;
      *) [[ -z "$file" ]] || die "arquivo repetido: $1"; file="$1"; shift ;;
    esac
  done
  [[ -n "$file" ]] || die "uso: setup.sh apply <topology.env> [--test] [--lock-password]"
  load_topology "$file"
  local target="$TOPO_TARGET" repo="$TOPO_REPO" dir="$TOPO_DIR"
  mkdir -p "$dir"

  info "executora $target, porta $TOPO_PORT"
  timeout 5 bash -c "exec 3<>/dev/tcp/$TOPO_HOST/$TOPO_PORT" 2>/dev/null ||
    die "a executora não responde na porta $TOPO_PORT. Nela, rode: sudo apt install -y openssh-server"

  require_planner_tools
  authorize_key "$target" "$TOPO_PORT" "$TOPO_KEY" "$dir/.state"
  require_remote_repo "$target" "$repo"

  info "setup da executora (pode pedir a senha do sudo de $TOPO_USER)"
  # The remote command is built and quoted here on purpose.
  # shellcheck disable=SC2029
  ssh "${SSH_OPTS[@]}" "${TTY_OPTS[@]}" "$target" \
    "cd $(remote_quote "$repo") && BRIDGE_APPLY=1 tools/agent-bridge/setup.sh executor"
  local name
  for name in "${apply_agent_keys[@]}"; do
    if [[ -n "${!name:-}" ]]; then
      ssh "${SSH_OPTS[@]}" -o BatchMode=yes "$target" \
        "$(remote_quote "$repo/tools/agent-bridge/setup.sh" env-set "$name" "${!name}")"
    fi
  done

  info "planejador (esta máquina)"
  install_sshfs "$dir/.state"
  fetch_scripts "$target" "$repo" "$dir"
  write_planner_env "$dir" "$TOPO_HOST" "$TOPO_USER" "$TOPO_PORT" "$TOPO_KEY" "$repo"
  if [[ -n "${PLANNER_MOUNT_DIR:-}" ]]; then
    set_env_value "$dir/.env" PLANNER_MOUNT_DIR "$PLANNER_MOUNT_DIR" 1
  fi
  if [[ -n "${PLANNER_MOUNT_READONLY:-}" ]]; then
    set_env_value "$dir/.env" PLANNER_MOUNT_READONLY "$PLANNER_MOUNT_READONLY" 1
  fi

  info "conferência"
  "$dir/planner.sh" check

  if ((run_test)); then
    info "teste: montagem e uma tarefa curta"
    "$dir/planner.sh" mount || warn "a montagem falhou; o resto da ponte funciona sem ela"
    local answer
    if ! answer="$(printf 'Responda apenas: ok. Não altere nenhum arquivo.\n' | "$dir/planner.sh" task)"; then
      printf '%s\n' "$answer"
      die "a tarefa de teste falhou"
    fi
    printf '%s\n' "$answer"
  fi

  if ((lock)); then
    info "desligando o login por senha na executora (pode pedir a senha do sudo)"
    # The remote command is built and quoted here on purpose.
    # shellcheck disable=SC2029
    ssh "${SSH_OPTS[@]}" "${TTY_OPTS[@]}" "$target" "$(remote_quote "$repo/tools/agent-bridge/setup.sh" lock-password)"
    ssh "${SSH_OPTS[@]}" -o BatchMode=yes "$target" true ||
      die "a chave deixou de entrar depois do lock-password; confira na executora antes de fechar qualquer sessão"
    info "a chave continua entrando"
  fi

  info "pronto: topologia de $file aplicada"
}

# env_set writes one allowed agent setting into this executor's .env, used by apply over SSH.
env_set() {
  (($# == 2)) || die "uso: setup.sh env-set <chave> <valor>"
  local allowed=" ${apply_agent_keys[*]} "
  [[ "$allowed" == *" $1 "* ]] || die "chave não permitida: $1"
  ensure_env_file "$script_dir/.env" "$script_dir/.env.example"
  set_env_value "$script_dir/.env" "$1" "$2" 1
  info "$1 gravado no .env da executora"
}

# lock_password turns off SSH password login once the planner key is authorized.
lock_password() {
  (($# == 0)) || die "uso: setup.sh lock-password"
  [[ -s "$HOME/.ssh/authorized_keys" ]] || die "nenhuma chave autorizada em ~/.ssh/authorized_keys; prepare o planejador antes"
  grep -q '^Include /etc/ssh/sshd_config.d/' /etc/ssh/sshd_config ||
    die "/etc/ssh/sshd_config não inclui sshd_config.d; ajuste PasswordAuthentication à mão"
  printf 'PasswordAuthentication no\nKbdInteractiveAuthentication no\n' | as_root tee "$sshd_dropin" >/dev/null
  as_root /usr/sbin/sshd -t
  restart_ssh
  info "login por senha desligado. Antes de fechar este terminal, confira que a chave entra: planner.sh check"
}

# remove_executor undoes on this machine what the bridge setup changed, keeping the agent and the repository.
remove_executor() {
  local key_line=""
  while (($# > 0)); do
    case "$1" in
      --key-line) key_line="${2:?}"; shift 2 ;;
      *) die "uso: setup.sh executor-remove [--key-line 'chave pública']" ;;
    esac
  done
  local state="$script_dir/.state" auth="$HOME/.ssh/authorized_keys" rule installed tmp
  info "removendo a ponte desta máquina executora"

  if [[ -n "$key_line" && -f "$auth" ]] && grep -qxF -- "$key_line" "$auth"; then
    tmp="$(mktemp)"
    grep -vxF -- "$key_line" "$auth" >"$tmp" || true
    cat "$tmp" >"$auth"
    rm -f "$tmp"
    info "chave do planejador retirada de $auth"
  elif [[ -z "$key_line" ]]; then
    warn "sem --key-line: a chave do planejador continua em $auth"
  fi

  rule="$(state_get "$state" STATE_UFW_RULE)"
  if [[ -n "$rule" ]] && command -v ufw >/dev/null 2>&1; then
    as_root ufw delete allow from "$rule" to any port 22 proto tcp || warn "não consegui remover a regra do firewall para $rule"
    info "regra do firewall para $rule removida"
  fi

  installed="$(state_get "$state" STATE_INSTALLED_OPENSSH)"
  if [[ -f "$sshd_dropin" ]]; then
    as_root rm -f "$sshd_dropin"
    info "login por senha volta à configuração padrão do SSH"
    if [[ "$installed" != "1" ]]; then
      restart_ssh || warn "não consegui reiniciar o SSH"
    fi
  fi

  rm -f "$script_dir/.env" "$state" "${XDG_RUNTIME_DIR:-/tmp}/radar-agent-bridge.lock"
  info ".env e registro da ponte apagados; os scripts continuam no repositório, que é versionado"

  if [[ "$installed" == "1" ]]; then
    info "removendo o servidor SSH, que foi instalado pelo setup"
    as_root apt-get remove -y -qq openssh-server || warn "não consegui remover o openssh-server"
  fi
}

# remove_planner undoes on this machine what the bridge setup changed: mount, key, scripts and sshfs.
remove_planner() {
  local dir="$1" host="$2" port="$3" key="$4" state mount_dir file
  state="$dir/.state"
  info "removendo a ponte deste planejador"

  if [[ -x "$dir/planner.sh" && -f "$dir/.env" ]]; then
    "$dir/planner.sh" unmount || warn "não consegui desmontar a pasta"
    mount_dir="$(expand_home "$(env_get "$dir/.env" PLANNER_MOUNT_DIR)")"
    if [[ -n "$mount_dir" ]]; then
      rmdir "$mount_dir" 2>/dev/null || true
    fi
  fi

  if [[ "$(state_get "$state" STATE_CREATED_KEY)" == "1" ]]; then
    rm -f "$key" "$key.pub"
    info "chave $key apagada"
  fi
  if [[ "$port" == "22" ]]; then
    ssh-keygen -R "$host" >/dev/null 2>&1 || true
  else
    ssh-keygen -R "[$host]:$port" >/dev/null 2>&1 || true
  fi

  if [[ "$(state_get "$state" STATE_INSTALLED_SSHFS)" == "1" ]]; then
    info "removendo o sshfs, que foi instalado pelo setup"
    as_root apt-get remove -y -qq sshfs || warn "não consegui remover o sshfs"
  fi

  for file in "${bridge_files[@]}" .env .state; do
    rm -f "${dir:?}/$file"
  done
  rmdir "$dir" 2>/dev/null || true
  info "scripts e .env do planejador apagados de $dir"
}

# setup_remove removes the bridge from both machines described by a topology file.
setup_remove() {
  local file="" skip_executor=0
  while (($# > 0)); do
    case "$1" in
      --skip-executor) skip_executor=1; shift ;;
      -*) die "opção desconhecida: $1" ;;
      *) [[ -z "$file" ]] || die "arquivo repetido: $1"; file="$1"; shift ;;
    esac
  done
  [[ -n "$file" ]] || die "uso: setup.sh remove <topology.env> [--skip-executor]"
  load_topology "$file"
  local target="$TOPO_TARGET" key_line=""

  if [[ -x "$TOPO_DIR/planner.sh" && -f "$TOPO_DIR/.env" ]]; then
    "$TOPO_DIR/planner.sh" unmount || warn "não consegui desmontar a pasta"
  fi

  if ((skip_executor == 0)); then
    if [[ -f "$TOPO_KEY.pub" ]]; then
      key_line="$(head -n 1 "$TOPO_KEY.pub")"
    fi
    SSH_OPTS=(-p "$TOPO_PORT" -i "$TOPO_KEY" -o IdentitiesOnly=yes)
    if ssh "${SSH_OPTS[@]}" -o BatchMode=yes "$target" true 2>/dev/null; then
      info "executora $target (pode pedir a senha do sudo de $TOPO_USER)"
      # The remote command is built and quoted here on purpose.
      # shellcheck disable=SC2029
      ssh "${SSH_OPTS[@]}" "${TTY_OPTS[@]}" "$target" \
        "$(remote_quote "$TOPO_REPO/tools/agent-bridge/setup.sh" executor-remove --key-line "$key_line")" ||
        warn "a remoção na executora terminou com erro; confira lá com: tools/agent-bridge/setup.sh executor-remove"
    else
      warn "não consegui entrar em $target; rode lá: tools/agent-bridge/setup.sh executor-remove"
    fi
  fi

  remove_planner "$TOPO_DIR" "$TOPO_HOST" "$TOPO_PORT" "$TOPO_KEY"
  info "pronto. O arquivo de topologia $file foi mantido; apague-o se não for mais usar"
}

# usage prints the available modes.
usage() {
  cat <<'EOF'
uso: setup.sh <modo>

No planejador, a partir do arquivo de topologia:
  apply topology.env [opções]     prepara as duas máquinas, entrando na executora por SSH
      --test                      no fim, monta a pasta e roda uma tarefa curta de teste
      --lock-password             no fim, desliga o login por senha na executora
  remove topology.env [opções]    remove a ponte das duas máquinas (não remove o agente)
      --skip-executor             remove só deste planejador

Em uma máquina só:
  executor                        prepara esta máquina como executora (de dentro do repositório)
  planner usuario@host [opções]   prepara esta máquina como planejadora
      --repo caminho              projeto na máquina executora (padrão: /home/usuario/radar-finance)
      --port 22                   porta SSH da máquina executora
      --key arquivo               chave SSH (padrão: ~/.ssh/radar_executor)
      --dir pasta                 onde ficam os scripts (padrão: a pasta deste script)
  lock-password                   desliga o login por senha nesta máquina executora
  executor-remove [--key-line k]  remove a ponte desta máquina executora
EOF
}

# main dispatches to the chosen mode.
main() {
  local mode="${1:-}"
  shift || true
  case "$mode" in
    apply) setup_apply "$@" ;;
    remove) setup_remove "$@" ;;
    executor) setup_executor "$@" ;;
    planner) setup_planner "$@" ;;
    lock-password) lock_password "$@" ;;
    executor-remove) remove_executor "$@" ;;
    env-set) env_set "$@" ;;
    *) usage; exit 1 ;;
  esac
}

main "$@"; exit
