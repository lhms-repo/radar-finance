# Radar

Sistema web de gestão financeira de um casal, hospedado em casa e acessado só pela rede local. Backend em Go, frontend em React, banco SQLite, entregue como um único binário num contêiner Docker.

## Fonte da verdade

- `docs/requisitos.md` é a especificação completa. Leia a seção relevante antes de qualquer tarefa.
- Requisitos têm códigos: RF para funcionais, RN para regras de negócio, RNF para não funcionais. Cite o código em commits, testes e comentários quando fizer sentido.
- `docs/referencias/` tem a imagem de referência do dashboard e o vídeo de animações. Siga esse visual.
- Se um requisito estiver ambíguo ou parecer conflitar com outro, pare e pergunte. Não invente comportamento de regra financeira.
- O dono do projeto toma as decisões finais. Proponha alternativas quando achar melhor, mas não mude uma decisão registrada no documento sem confirmação.

## Forma de trabalhar

- Trabalhe uma fase por vez, na ordem da seção 9 do documento. Não comece a fase seguinte sem a anterior aprovada.
- No início de cada fase, apresente um plano curto com o que será feito e os arquivos principais, e espere aprovação.
- Para cada regra de negócio da fase, escreva primeiro os testes em tabela com `go test`, depois a implementação.
- Ao terminar uma fase, rode todos os testes e o lint, e resuma o que foi entregue frente ao critério de pronta da fase.
- Commits pequenos, em português, no formato `fase N: descrição`.

## Stack

### Backend
- Go, só a biblioteca padrão `net/http` para roteamento; chi apenas se realmente necessário.
- SQLite em modo WAL com o driver `modernc.org/sqlite`, sem CGO.
- sqlc para consultas tipadas a partir de SQL escrito à mão.
- goose para migrações, executadas automaticamente na inicialização.
- scs para sessões. Sem senhas: dois usuários fixos, escolhidos na tela "Quem está usando?" (RF-01).
- OpenAPI como contrato único: `api/openapi.yaml`, com oapi-codegen gerando o servidor.
- Backup diário com `VACUUM INTO`, agendado no próprio serviço, com recuperação do backup perdido na inicialização.

### Frontend
- React com TypeScript e Vite, gerenciado com pnpm.
- Tailwind CSS com shadcn/ui; tema claro e escuro desde o início.
- Motion para animações, NumberFlow para a rolagem de dígitos dos valores, Recharts para gráficos.
- TanStack Query para dados, React Hook Form com Zod para formulários.
- Tipos da API gerados com openapi-typescript a partir do mesmo `openapi.yaml`.
- PWA instalável pelo Safari do iPhone.

### Testes
- `go test` com testes em tabela para todas as regras de negócio.
- Playwright para fluxos de ponta a ponta, sempre incluindo o WebKit com viewport de iPhone.

## Regras invioláveis

- Dinheiro é sempre `int64` em centavos, do banco até a API. Nunca `float`. Formatação para reais só na interface.
- Toda gravação é transacional.
- Nenhuma regra de cálculo financeiro no frontend. O backend calcula; o frontend exibe.
- Datas de negócio em `America/Sao_Paulo`.
- Interface em português do Brasil, valores em reais e datas no formato dia, mês e ano.
- Código sempre em inglês: nomes de variáveis, funções, tipos, pacotes, arquivos, tabelas, colunas, rotas da API e comentários. Só os textos exibidos ao usuário ficam em português.
- Toda função, exportada ou não, tem logo acima um comentário curto e direto, de uma linha sempre que possível, dizendo o que ela faz. Em Go, siga a convenção de começar pelo nome da função, como `// CalculateInvoiceTotal returns the invoice total in cents.`. Em TypeScript, use um comentário `/** ... */` de uma linha.
- Nunca use travessão, nem em código, comentários, mensagens, documentação ou commits. Prefira vírgula, dois pontos ou uma frase nova.
- Toda animação respeita `prefers-reduced-motion`.
- Nunca grave dados reais de exemplo no repositório. Use dados fictícios nos testes e nas seeds.

## Estado atual

- Fase 1 (Fundação) entregue e em produção desde 10/11/2026, aguardando aprovação do dono para começar a Fase 2.
- Entrada sem senha com dois usuários fixos (RF-01 reescrito por decisão do dono). Os nomes reais ficam só no banco de produção.
- Produção nesta máquina Windows: `https://<nome-do-pc>.local` e IPs reservados no roteador, definidos em `deploy/.env` (fora do git). Certificados em `deploy/certs/`, com a autoridade local em `deploy/certs/ca`, que nunca sai do PC. Para regenerar o certificado, reutilize a mesma autoridade, senão os iPhones deixam de confiar.
- POC da NFC-e validada com uma nota real de Goiás em 11/11/2026. O link do QR code devolve uma casca com iframe; o iframe, buscado com o mesmo cookie, traz o DANFE como string em `new DanfeNFCe(...)`, no layout do portal nacional. Para a Fase 7: preço unitário pode ter três casas decimais (combustível) e hoje é truncado em centavos.
- Pendente: login automático e energia do Windows (seção 2 de `docs/instalacao.md`).
- Fase 2 em andamento: etapas 2.1 (categorias), 2.2 (contas), 2.3 (rendas), 2.4 (cartões e faturas) e 2.5 (lançamento rápido) entregues; falta 2.6 (fechamento e conferência com a fatura do Nubank). Lançamentos de conta ficam em `entries` com valor com sinal; compras no cartão viram `installments` e só o pagamento da fatura toca a conta (RN-07). Rendas previstas e cobranças de recorrência são derivadas do cadastro, não gravadas mês a mês.
- Atualize esta seção ao fim de cada fase.

## Plataforma

- Única plataforma móvel: iPhone com Safari. Não é preciso suportar Android.
- Produção roda em Docker Desktop no Windows com WSL2. O repositório fica dentro do sistema de arquivos do WSL2.
- Acesso apenas pela rede local, por HTTPS com Caddy e certificado gerado com mkcert.

## Estrutura sugerida

```
cmd/radar/          ponto de entrada do binário
internal/           domínio, serviços, regras de negócio e acesso a dados
internal/db/        migrações, consultas SQL e código gerado pelo sqlc
api/openapi.yaml    contrato da API
web/                frontend React
deploy/             Dockerfile de produção, docker-compose.yml e Caddyfile
docs/               requisitos e referências visuais
.devcontainer/      ambiente de desenvolvimento
```

## Comandos

Mantenha um `Makefile` com pelo menos: `make dev`, `make test`, `make lint`, `make generate` e `make build`. Documente qualquer comando novo aqui.

Dentro do Dev Container:

| Comando | O que faz |
| --- | --- |
| `make setup` | Baixa dependências Go e pnpm e os navegadores do Playwright |
| `make generate` | Gera sqlc, servidor oapi-codegen e tipos do front a partir de `api/openapi.yaml` |
| `make dev` | API Go na porta 8080 e Vite na 5173, com banco em `tmp/` |
| `make user-add LOGIN=x NAME="Y"` | Cria usuário no banco de desenvolvimento |
| `make test` | `go test ./...` e checagem de tipos do front |
| `make e2e` | Playwright com WebKit de iPhone, WebKit e Chromium de desktop. Os três projetos dividem o mesmo banco, então os testes usam nomes com o projeto |
| `make lint` | golangci-lint e ESLint |
| `make build` | Front e binário único em `bin/radar` |
| `make icons` | Gera os ícones do PWA a partir de `web/icon.svg` |
| `make certs HOSTS="nome.local 192.168.x.y"` | Autoridade local e certificado HTTPS com mkcert |

No terminal do WSL2, fora do Dev Container:

| Comando | O que faz |
| --- | --- |
| `make deploy` | Constrói e sobe a produção (RNF-19) |
| `make deploy-user-add LOGIN=x NAME="Y"` | Cria usuário na produção |
| `make deploy-backup` | Backup manual na produção |
| `make deploy-logs` | Logs da produção |
| `deploy/restore.sh <arquivo>` | Restaura um backup, veja `docs/backup.md` |

Código gerado (`internal/db/store`, `internal/api/gen`, `web/src/api/schema.ts`) não é editado à mão e fica fora da regra de comentário por função.
