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
- scs para sessões. Entrada por grupo, nome e PIN (RF-01, RN-27), com o PIN guardado como hash PBKDF2 da biblioteca padrão; a sessão guarda a pessoa e o grupo ativo.
- OpenAPI como contrato único: `api/openapi.yaml`, com oapi-codegen gerando o servidor.
- Backup diário com `VACUUM INTO`, agendado no próprio serviço, com recuperação do backup perdido na inicialização.

### Frontend
- React com TypeScript e Vite, gerenciado com pnpm.
- Tailwind CSS com shadcn/ui; tema claro e escuro desde o início.
- Motion para animações, NumberFlow para a rolagem de dígitos dos valores, Recharts para gráficos, jsQR para ler o QR code da NFC-e pela câmera.
- TanStack Query para dados, React Hook Form com Zod para formulários.
- Tipos da API gerados com openapi-typescript a partir do mesmo `openapi.yaml`.
- PWA instalável pelo Safari do iPhone e pelo Chrome do Android.

### Testes
- `go test` com testes em tabela para todas as regras de negócio.
- Playwright para fluxos de ponta a ponta, sempre incluindo o WebKit com viewport de iPhone e, a partir da Fase 12, o Chromium com viewport de Android.

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
- Até a Fase 7, entrada sem senha com dois usuários fixos. A partir da Fase 8, grupos de pessoas com PIN (RF-01 reescrito em 12/11/2026). Os nomes reais ficam só no banco de produção.
- Produção nesta máquina Windows: `https://<nome-do-pc>.local` e IPs reservados no roteador, definidos em `deploy/.env` (fora do git). Certificados em `deploy/certs/`, com a autoridade local em `deploy/certs/ca`, que nunca sai do PC. Para regenerar o certificado, reutilize a mesma autoridade, senão os celulares deixam de confiar.
- NFC-e (`internal/nfce`, antes a POC de `tools/nfce-poc`): o link do QR code de Goiás devolve uma casca com iframe; o iframe, buscado com o mesmo cookie, traz o DANFE como string em `new DanfeNFCe(...)`, no layout do portal nacional. Só endereços `.gov.br` são baixados. Preço unitário com três casas é truncado em centavos (R$ 6,499 vira R$ 6,49) e quantidade fica em milésimos.
- Pendente: login automático e energia do Windows (seção 2 de `docs/instalacao.md`).
- Fases 1 a 7 entregues e aprovadas em 11/11/2026; produção em `928f78e`. Próximo passo: as conferências com dados reais listadas abaixo, feitas com o dono em produção. O repositório está em `git@github.com:italogfx/radar-finance.git`, com chave SSH do WSL; push só quando o dono pedir. O Mercado (`internal/market`) tem mercados (com o CNPJ vindo da nota), produtos sempre de marca, com tamanho em milésimos e `by_weight` para itens por kg, preços com histórico (`prices`, origem manual ou nota), notas (`receipts`, chave de acesso única, apagar a nota apaga os preços dela) e vínculos por mercado (`receipt_links`, pelo código da linha ou pelo texto normalizado, ou ignorar). Listas (`shopping_lists`, `list_items` com quantidade em milésimos) são compartilhadas ao vivo: toda gravação bem-sucedida na API publica um evento em `internal/events` e `/api/events` (SSE, fora do middleware de sessão, que bufferiza) faz a outra tela recarregar. A comparação usa o preço mais recente de mercados ativos; ranking com completos primeiro; custo de item por peso truncado; melhor divisão passo a passo (RN-24); preço desatualizado depois de `stale_days`. Finalizar grava o gasto em Compras para casa (conta ou cartão) ou a compra no Alelo e fecha a lista na mesma transação, usando `AddExpenseTx`, `AddPurchaseTx` dos serviços. A importação (`internal/importer`) lê o CSV do Nubank (`date,title,amount`, valores como `"1.234,56"` ou `1234.56`), aplica a regra de apelido mais recente para o texto normalizado (minúsculas, espaços simples), marca como duplicata a parcela com mesmo número, total de parcelas e valor na fatura, a linha à vista com mesmo valor e data de compra a um dia de distância no cartão, ou com o valor de uma recorrência da fatura, e grava tudo na fatura escolhida, com `purchases.import_id` para desfazer. O pagamento da fatura no CSV é ignorado (RN-07); estorno vira crédito; parcela de compra que não existe no Radar é criada só da parcela do CSV em diante. Linha sem categoria cai na subcategoria `system_key = 'uncategorized'`. Indicadores ficam em `internal/report` (gasto por categoria só com a cota do casal, orçamento, saldo previsto por grupo, moradia, comprometimento, fechamento, patrimônio, busca) e avisos em `internal/notify`, derivados do estado com leitura por usuário. Moradia tem `system_key = 'housing'`. Compras divididas guardam a cota total de cada pessoa (`splits`) e a cota por parcela (`split_installments`); o recebimento é uma entrada `receivable`, que não é renda. Caixinhas: aporte e retirada são entradas `savings_deposit` e `savings_withdrawal` numa conta, fora de gasto e renda; percentual em pontos-base. Lançamentos de conta ficam em `entries` com valor com sinal; compras no cartão viram `installments` e só o pagamento da fatura toca a conta (RN-07). Rendas, recorrências, contas fixas, parcelas de financiamento e créditos do Alelo são derivados do cadastro mês a mês; confirmar ou pagar grava o lançamento. A agenda (`internal/agenda`) junta contas fixas, parcelas e faturas do mês. A subcategoria Compras para casa tem `system_key = 'groceries'` (RN-15, RN-26) e não pode ser excluída.
- O dono só começa a usar o Radar com todas as fases prontas. Conferências com dados reais ficam para o fim: Fase 2, a fatura do mês no Radar bater com a fatura real do Nubank; Fase 3, todas as contas da casa e os quatro financiamentos no calendário com saldo devedor correto; Fase 4, a compra dividida do ar-condicionado com cotas, pagamentos e parte líquida corretos; Fase 5, o saldo previsto do mês conferindo com a conta feita à mão; Fase 6, uma fatura real do Nubank importada sem duplicatas e com os apelidos aplicados (formato conferido com um CSV real em 11/11/2026: valores entre aspas no formato brasileiro, "- " antes do pagamento, parcela de Pix só com " - N/M" e, da segunda parcela em diante, a data do início do ciclo no lugar da data da compra); Fase 7, uma lista real mostrando ranking e melhor divisão entre mercados a partir de notas escaneadas (a leitura de nota real em produção ainda não foi feita fora da POC).
- Fase 8 (Grupos e entrada) entregue em 12/11/2026 e aprovada em 13/11/2026. Fase 9 (Contas da pessoa e lançamentos pessoais) entregue e aprovada em 13/11/2026. Fase 10 (Acerto entre membros) entregue e aprovada em 13/11/2026. Fase 11 (Textos e ponta a ponta) entregue e aprovada em 13/11/2026. Fase 12 (Android) entregue e aprovada em 13/11/2026, conferida no navegador do app imitando um Android; falta a conferência num Android de verdade (instalação pelo Chrome, que pode pedir um service worker, certificado e câmera). O voltar do Android fecha a camada aberta antes de sair da tela por `useCloseOnBack` em `web/src/lib/back.ts` (RNF-26), usado no `Dialog`, na folha Mais, no leque e no menu do Adicionar, nos avisos e nos menus (`MenuRoot`): abrir põe um passo no histórico, voltar fecha, e fechar pela tela tira o passo; a folha Mais navega com `replace`. A vibração usa `navigator.vibrate` quando existe e o interruptor do iOS no iPhone. A cor da barra do sistema segue o tema escolhido no app (`applyThemeColor` e `public/theme-init.js`). O acerto (`internal/settle`) é calculado na leitura, desde `groups.settles_from` (RN-31): gastos de conta do grupo (contas fixas e financiamentos pagos entram por aqui), parcelas e recorrências do cartão no fechamento da fatura (`card.GroupCharges`, só a parte do grupo, sem terceiros) e compras do Alelo; quem pagou é quem tem a conta, o cartão ou o Alelo. A cota usa os pesos (`group_weights`) e a divisão escolhida (`member_splits` com `member_split_shares`, em `internal/membersplit`) em vigor quando o gasto aconteceu, com histórico por `from_day` e `set_at`: no dia de uma mudança, só o que foi lançado depois dela (`created_at`) usa a nova (`membersplit.Applies`). Divisão por valor é exata para gasto avulso, compra e Alelo (compra pela parte gravada, `PurchaseGroupPart`) e proporção para conta fixa, financiamento e recorrência. Na API, `memberSplit` ausente numa edição mantém a divisão; `weights` explícito limpa. Compras anteriores ao início do acerto ficam fora. Pagamento de acerto só por quem paga, quem recebe ou administrador; titular de conta, cartão ou Alelo que pagou gastos de um grupo não muda. A cota de cada membro vai com os centavos para o maior resto e, no empate, para quem entrou antes. Gatilhos apagam a divisão quando o item é apagado ou muda de espaço. Pagamento de acerto é uma transferência (`ledger.TransferTx`) mais uma linha em `settlements`; desfazer apaga a transferência. A edição de um item lê a divisão por `GET /member-splits/{kind}/{id}`. O espaço Pessoal é um grupo com `kind = 'personal'` e `owner_person_id`, de um só membro, criado quando a pessoa entra no primeiro grupo (migração 22, em Go; `internal/db/schema/` espelha as colunas para o sqlc), fora da tela de entrada. A fatura de um cartão é lida em cada espaço só com as linhas dele; no Pessoal da dona ela vem completa, com o grupo de cada linha, e só ali é paga, inteira (`invoice_payments` e o lançamento do pagamento ficam no Pessoal). O saldo da conta e o uso do limite são reais, com todos os espaços, e a API só os mostra ao titular (`owned`); o extrato completo com saldos fica no Pessoal. O patrimônio do grupo não soma contas e a previsão do mês usa `InvoicePart`. Criar em outro espaço usa o cabeçalho `X-Radar-Space`, conferido no `memberOnly`; mover usa `POST /spaces/{id}/moves` (`internal/space`), só por quem lançou o item, leva os lançamentos ligados e recusa compra dividida ou conta, cartão e pessoa fora do destino. O front manda `X-Radar-Space` com o espaço da tela em toda chamada. O ajuste de saldo só é feito pela titular, no Pessoal dela; administrar grupos não vale no Pessoal, e o nome Pessoal é reservado. A importação procura duplicatas em todos os espaços do cartão. O grupo ativo fica em `sessions.group_id`, que só a troca de grupo escreve, porque o scs grava a sessão inteira ao fim de toda requisição. Ao subir a Fase 8 na produção, a migração cria o grupo Casa com todos como administradores, mas ninguém tem PIN: rode `make deploy-user-pin LOGIN=x` para cada pessoa logo depois do `make deploy`. A proposta "Radar em grupos" foi aprovada com as decisões: PIN por pessoa; uma pessoa em vários grupos; acerto entre membros só do que é marcado como do grupo, pelos pesos do grupo; catálogo do Mercado por grupo; membro administrador; um banco com `group_id` nas tabelas; contas e cartões da pessoa; lançamento pessoal visível só para o dono; acerto do casal começando na migração. Requisitos nas seções 2.15, 3 (RN-27 a RN-31) e 9 de `docs/requisitos.md`. Fora da v1 (seção 10 dos requisitos): sair de grupo com saldo de acerto, categorias por grupo e rendas pessoais na base das caixinhas. Em aberto: exportação por grupo e patrimônio por pessoa ou grupo. Provisório: mudar o dia de fechamento de um cartão vale para todos os grupos; qualquer pessoa na rede pode bloquear o PIN de outra por 15 minutos errando cinco vezes (RN-27). Na Fase 11, os textos de tela falam do grupo pelo nome, ou com a pessoa no Pessoal (`useSpaceWords` em `web/src/app/space.tsx`), e o e2e `web/e2e/twogroups.spec.ts` confere dois grupos lendo a mesma nota e todas as telas sem a palavra casal. O Alelo segue a conta: o saldo real (todas as compras) só vai ao titular (`owned`), o mês completo com crédito, saldos e o grupo de cada compra só no Pessoal do titular (`ledger.HolderSpace`), onde o crédito é confirmado; o grupo vê só as compras dele. Recorrências do cartão e compras do Alelo têm o campo Onde; a compra do Alelo se move (`kind = 'alelo'` em `internal/space`, deixando a divisão entre membros para trás pela migração 25), a recorrência não. Os PRs vão para o `origin` deste clone, `lhms-repo/radar-finance`.
- Atualize esta seção ao fim de cada fase.

## Plataforma

- Plataformas móveis: iPhone com Safari e Android com Chrome (RNF-20, Android incluído em 13/11/2026 e entregue pela Fase 12). No Android, o acesso é pelo IP reservado no roteador.
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
| `make user-add LOGIN=x NAME="Y" [GROUP="Z"]` | Cria usuário no banco de desenvolvimento, opcionalmente já num grupo |
| `make group-create NAME="Z" ADMIN=x` | Cria um grupo de desenvolvimento com o primeiro administrador |
| `make user-pin LOGIN=x` | Define o PIN de um usuário de desenvolvimento, digitado no terminal |
| `make test` | `go test ./...` e checagem de tipos do front |
| `make e2e` | Playwright com WebKit de iPhone, WebKit e Chromium de desktop e Chromium de Android (Pixel 7). Os quatro projetos dividem o mesmo banco, então os testes usam nomes com o projeto. As notas fiscais vêm das páginas fictícias de `internal/nfce/testdata` pela variável `RADAR_NFCE_FIXTURES`, nunca da Sefaz |
| `make lint` | golangci-lint e ESLint |
| `make build` | Front e binário único em `bin/radar` |
| `make icons` | Gera os ícones do PWA a partir de `web/icon.svg` |
| `make certs HOSTS="nome.local 192.168.x.y"` | Autoridade local e certificado HTTPS com mkcert |

No terminal do WSL2, fora do Dev Container:

| Comando | O que faz |
| --- | --- |
| `make deploy` | Constrói e sobe a produção (RNF-19) |
| `make deploy-user-add LOGIN=x NAME="Y" [GROUP="Z"]` | Cria usuário na produção, opcionalmente já num grupo |
| `make deploy-group-create NAME="Z" ADMIN=x` | Cria um grupo na produção com o primeiro administrador |
| `make deploy-user-pin LOGIN=x` | Define o PIN de um usuário na produção, digitado no terminal |
| `make deploy-backup` | Backup manual na produção |
| `make deploy-logs` | Logs da produção |
| `deploy/restore.sh <arquivo>` | Restaura um backup, veja `docs/backup.md` |

Código gerado (`internal/db/store`, `internal/api/gen`, `web/src/api/schema.ts`) não é editado à mão e fica fora da regra de comentário por função.
