# Ponte entre agentes: planejador e executor

Dois agentes de código trabalham em máquinas diferentes. O **planejador** escreve cada tarefa, envia por SSH, lê o projeto por uma pasta montada com `sshfs` e revisa rodando comandos pela mesma conexão. O **executor** implementa, na única cópia do projeto. Na máquina executora nada do planejador é instalado.

Hoje o planejador é o Claude Code e o executor é o Codex CLI, mas os nomes dos arquivos, dos comandos e das variáveis não dependem disso. Só dois pontos são do Codex, porque é ele quem os lê: o binário padrão `codex`, com `codex exec`, e a variável `CODEX_HOME`, preenchida por `AGENT_HOME_DIR`.

Toda a configuração fica num `.env` com permissão 600 em cada máquina. Os scripts recusam o arquivo se a permissão não for 600 ou se ele não pertencer ao usuário que roda o script.

```mermaid
flowchart TB
  subgraph A["Máquina do planejador"]
    C["Agente planejador"]
    BR["planner.sh + .env (600)"]
    M["Pasta montada, só leitura"]
  end
  subgraph B["Máquina executora"]
    SV["executor.sh + .env (600)"]
    X["Agente executor, sem interação"]
    R["Pasta do projeto, única cópia"]
  end
  C -->|"task e reply"| BR
  BR -->|"ssh, pedido na entrada padrão"| SV
  SV --> X
  X -->|"lê e edita"| R
  SV -->|"sessão e resposta final"| BR
  R -.->|"sshfs"| M
  M -->|"contexto e revisão"| C
  BR -->|"run: git diff, make test"| R
```

## Por que não MCP

O Codex removeu o comando `codex mcp-server` na versão 0.154.0. O substituto oficial, o app server, fala um protocolo próprio, e o plugin do Codex para o Claude Code exige os dois na mesma máquina. Por isso a ponte usa o `codex exec`, o modo sem interação, que devolve a resposta final e o identificador da sessão para continuar a conversa.

## Arquivos

| Arquivo | Onde roda | Para quê |
| --- | --- | --- |
| `AGENTS.md` | executora | Regras do projeto e formato da resposta para o executor |
| `tools/agent-bridge/executor.sh` | executora | Carrega o `.env`, entra no projeto e roda o agente sem interação, uma tarefa por vez |
| `tools/agent-bridge/planner.sh` | planejador | Envia tarefas, continua sessões, roda comandos de revisão e monta a pasta |
| `tools/agent-bridge/lib.sh` | as duas | Leitura do `.env` sem executá-lo e checagem da permissão |
| `tools/agent-bridge/.env.example` | as duas | Modelo do `.env`, com as variáveis das duas máquinas |

O `.env` é lido literalmente: sem expansão de `$VARIAVEL` e sem comentário depois do valor. Use caminhos absolutos; só `~` no início é aceito nos caminhos locais.

## 1. Máquina executora

A pasta `tools/agent-bridge/` fica dentro do repositório: o `executor.sh` descobre a pasta do projeto pela própria localização. Os passos supõem o PC com Windows e o projeto no WSL, como a produção.

| Passo | Como fazer | Feito |
| --- | --- | --- |
| Agente instalado e autenticado | No WSL, `codex login` com a conta do ChatGPT, ou o modelo local configurado em `~/.codex/config.toml` | [ ] |
| Servidor SSH no WSL | `sudo apt install openssh-server` e `sudo systemctl enable --now ssh` | [ ] |
| WSL visível na rede local | Em `%UserProfile%\.wslconfig`, seção `[wsl2]`, a linha `networkingMode=mirrored`; depois `wsl --shutdown` no PowerShell | [ ] |
| Firewall | No PowerShell como administrador: `New-NetFirewallRule -DisplayName "Radar SSH executor" -Direction Inbound -Protocol TCP -LocalPort 22 -Profile Private -Action Allow` | [ ] |
| `.env` | `cp tools/agent-bridge/.env.example tools/agent-bridge/.env`, `chmod 600 tools/agent-bridge/.env` e preencher a parte "Executor machine" | [ ] |
| Teste local | `tools/agent-bridge/executor.sh --check` mostra a pasta, a versão e o login do agente, e `ok` para go, pnpm, node, make, git e flock | [ ] |

Se a conexão da outra máquina não chegar ao WSL mesmo com a rede espelhada, confira na documentação do WSL as regras de firewall do Hyper-V para a rede espelhada.

Numa sessão SSH sem terminal, o `.bashrc` do Ubuntu para logo no início, então o `PATH` do nvm, do Go e do pnpm pode não existir. Por isso o `.env` tem `AGENT_BIN` e `AGENT_PATH_EXTRA`.

## 2. Máquina do planejador

Precisa de bash, cliente SSH, `sshfs` e o agente planejador. O `sshfs` é nativo no Linux, pede o macFUSE no macOS e, no Windows, fica mais simples dentro do WSL. A pasta `agent-bridge` fica fora de qualquer repositório.

| Passo | Como fazer | Feito |
| --- | --- | --- |
| Chave SSH | `ssh-keygen -t ed25519 -f ~/.ssh/radar_executor -C radar-executor`. Sem senha, ou com senha guardada no `ssh-agent` | [ ] |
| Autorizar a chave | `ssh-copy-id -i ~/.ssh/radar_executor.pub usuario@maquina-executora` | [ ] |
| Copiar os scripts | `scp -r usuario@maquina-executora:/caminho/do/projeto/tools/agent-bridge ~/agent-bridge` | [ ] |
| `.env` | `cp ~/agent-bridge/.env.example ~/agent-bridge/.env`, `chmod 600 ~/agent-bridge/.env` e preencher a parte "Planner machine" | [ ] |
| Conferir | `~/agent-bridge/planner.sh check` | [ ] |
| Montar a pasta | `~/agent-bridge/planner.sh mount` | [ ] |

Abra o agente planejador dentro da pasta montada (`~/radar-remote` por padrão). Assim ele lê as instruções e o restante do projeto direto da máquina executora.

## 3. Opcional: chave só para as tarefas

Para que a chave usada nas tarefas não abra um shell na máquina executora:

1. Gere uma segunda chave, por exemplo `~/.ssh/radar_executor_tools`, e autorize-a normalmente. Ela serve para o `run` e o `mount`.
2. Na máquina executora, em `~/.ssh/authorized_keys`, coloque antes da chave das tarefas: `command="/caminho/do/projeto/tools/agent-bridge/executor.sh",restrict`.
3. No `.env` da máquina do planejador: `EXECUTOR_SSH_KEY` com a chave restrita e `EXECUTOR_SSH_TOOLS_KEY` com a segunda.

Com isso, a chave restrita só consegue rodar `exec`, `resume <sessão>` e `--check`. Qualquer outro comando recebe a mensagem de uso.

## 4. Uso

```bash
# Tarefa nova: o pedido vai num arquivo local ou pela entrada padrão
~/agent-bridge/planner.sh task /tmp/pedido-0012.md

# Continuar a mesma sessão com uma correção
~/agent-bridge/planner.sh reply <sessão> /tmp/correcao-0012.md

# Revisão, sempre na máquina executora
~/agent-bridge/planner.sh run git status --short
~/agent-bridge/planner.sh run git diff
~/agent-bridge/planner.sh run make test
~/agent-bridge/planner.sh run make lint
```

O `task` e o `reply` respondem com três partes: `sessão: <id>`, `código de saída: <n>` e a resposta final do agente. Se não houver resposta final, aparecem os últimos erros. Só uma tarefa roda por vez: uma segunda chamada enquanto a primeira trabalha é recusada.

O `run` passa cada argumento literalmente, sem pipes nem `&&`.

No Claude Code, um comando de terminal pode durar no máximo 10 minutos. Para tarefas maiores, o planejador roda o `task` em segundo plano e lê o resultado quando ele termina.

No modo `workspace-write`, o sandbox do executor costuma bloquear a rede. Deixe as dependências instaladas antes (`make setup`) ou libere a rede na configuração do agente.

### Modelo de pedido ao executor

```markdown
Tarefa: <título curto> (<códigos RF/RN>)

Leia: CLAUDE.md, docs/requisitos.md seção <n>, <arquivos>.
Altere só: <arquivos>.

Passos:
1. <passo>
2. <passo>

Casos de teste (tabela em go test):
| entrada | esperado |
| ... | ... |

Fora do escopo: <o que não fazer>.

Pronto quando: make test e make lint passam. Responda no formato do AGENTS.md.
```

## 5. O que fica na máquina executora

- Processos do `sshd`, do agente, do `sftp-server` (da montagem) e dos comandos que o agente e o `run` executam.
- Logs do SSH.
- O histórico das sessões do agente (em `~/.codex/sessions/`, ou em `AGENT_HOME_DIR`), com todos os pedidos enviados pelo planejador.
