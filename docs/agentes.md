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
| `tools/agent-bridge/setup.sh` | as duas | Prepara cada máquina: `executor`, `planner` e `lock-password` |
| `tools/agent-bridge/lib.sh` | as duas | Leitura do `.env` sem executá-lo e checagem da permissão |
| `tools/agent-bridge/.env.example` | as duas | Modelo do `.env`, com as variáveis das duas máquinas |

O `.env` é lido literalmente: sem expansão de `$VARIAVEL` e sem comentário depois do valor. Use caminhos absolutos; só `~` no início é aceito nos caminhos locais.

## 1. Máquina executora

No WSL, de dentro do repositório (com o projeto em `/home/<usuario>/radar-finance`):

```bash
codex login            # se o agente ainda não tiver login
tools/agent-bridge/setup.sh executor
```

O `setup.sh executor`:

- instala e liga o servidor SSH;
- confere o login do agente e encontra `codex`, `go`, `pnpm` e `node`, porque numa sessão SSH sem terminal o `.bashrc` do Ubuntu não monta o `PATH`;
- cria o `.env` com permissão 600 e preenche `AGENT_BIN` e `AGENT_PATH_EXTRA`, sem apagar o que você já tiver escrito;
- roda o `executor.sh --check`;
- no fim, mostra os dois comandos para a máquina do planejador.

Se faltar `go` ou `pnpm`, instale-os no WSL nas versões do `.devcontainer` e rode o setup de novo. Hoje eles podem existir só dentro do Dev Container.

Se o WSL ainda não estiver em rede espelhada, o setup mostra os comandos para o PowerShell do Windows como administrador: a linha `networkingMode=mirrored` no `.wslconfig`, a regra de firewall da porta 22 e o `wsl --shutdown`. Se esse PC for a produção, o `wsl --shutdown` derruba o Docker e o Radar por alguns instantes. Se mesmo assim a conexão não chegar ao WSL, confira na documentação do WSL as regras de firewall do Hyper-V para a rede espelhada.

## 2. Máquina do planejador

Precisa de bash, cliente SSH e `tar`. Rode os dois comandos que o setup da executora mostrou, já com o usuário e o IP dela. Têm este formato:

```bash
ssh usuario@ip-da-executora 'tar --exclude=agent-bridge/.env -C /home/usuario/radar-finance/tools -cf - agent-bridge' | tar -C ~ -xf -
~/agent-bridge/setup.sh planner usuario@ip-da-executora
```

O primeiro comando copia os scripts, sem o `.env` da executora. O `setup.sh planner`:

- instala o `sshfs` quando há `apt-get`; no macOS, instale o macFUSE e o sshfs à mão;
- cria a chave `~/.ssh/radar_executor`, se ainda não existir;
- autoriza a chave na executora, pedindo a senha uma única vez;
- copia de novo os scripts da executora, o que também serve para atualizá-los depois;
- cria o `.env` com permissão 600, com host, usuário, porta, chave e pasta do projeto;
- roda o `planner.sh check`.

Opções: `--repo` para outra pasta do projeto, `--port`, `--key` e `--dir`.

Depois:

```bash
~/agent-bridge/planner.sh mount
echo "Responda apenas: ok. Não altere nenhum arquivo." | ~/agent-bridge/planner.sh task
```

Quando a tarefa de teste responder, desligue o login por senha na executora:

```bash
tools/agent-bridge/setup.sh lock-password
```

Antes de fechar o terminal da executora, confira com `planner.sh check` que a chave continua entrando.

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
