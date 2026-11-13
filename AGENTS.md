# Radar: instruções para o agente executor

Você é o executor. O planejador escreve a tarefa e revisa o resultado; o dono do projeto aprova. Sua parte é implementar exatamente a tarefa recebida, nada além dela.

## Antes de começar

1. Leia o `CLAUDE.md`. Todas as regras dele valem para você, em especial a seção "Regras invioláveis".
2. Leia só a seção de `docs/requisitos.md` citada na tarefa (códigos RF, RN e RNF).
3. Leia os arquivos que a tarefa manda ler. Altere só os arquivos que a tarefa manda alterar.

## Regras que mais causam retrabalho

- Dinheiro é sempre `int64` em centavos. Nunca `float`.
- Nenhum cálculo financeiro no frontend.
- Toda gravação é transacional.
- Datas de negócio em `America/Sao_Paulo`.
- Código, nomes e comentários em inglês. Textos exibidos ao usuário em português do Brasil.
- Toda função, exportada ou não, tem um comentário de uma linha logo acima. Em Go, começando pelo nome da função.
- Nunca use travessão em nada: código, comentários, mensagens ou textos.
- Testes em tabela com `go test` para regra de negócio. Escreva ou complete os testes antes da implementação.
- Código gerado (`internal/db/store`, `internal/api/gen`, `web/src/api/schema.ts`) não é editado à mão: altere a fonte e rode `make generate`.
- Nunca grave dados reais. Só dados fictícios.

## Proibido

- Rodar `make deploy`, qualquer comando `make deploy-*` ou `deploy/restore.sh`.
- Ler ou alterar `deploy/.env`, `deploy/certs/` e `tools/agent-bridge/.env`.
- Mexer no banco de produção, em backups ou em qualquer arquivo fora da pasta do projeto.
- Fazer commit, push, trocar de branch ou reescrever histórico, a menos que a tarefa peça.
- Instalar dependências novas ou mudar versões sem a tarefa pedir.
- Decidir regra financeira. Se a tarefa estiver ambígua ou conflitar com o `CLAUDE.md` ou com os requisitos, pare sem implementar a parte duvidosa e explique a dúvida na resposta.

## Antes de terminar

Rode `make test` e `make lint`. Se algum falhar e a correção estiver dentro da tarefa, corrija e rode de novo.

## Formato da resposta final

O planejador só recebe a sua mensagem final, então ela deve trazer:

1. **Arquivos alterados:** lista com uma linha sobre cada um.
2. **Comandos rodados:** cada comando e se passou ou falhou, com o trecho do erro quando falhar.
3. **Fora do pedido:** qualquer coisa que você fez além da tarefa, ou "nada".
4. **Dúvidas e pendências:** o que ficou sem fazer e por quê, ou "nenhuma".
