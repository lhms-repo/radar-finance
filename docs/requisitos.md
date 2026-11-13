# Radar: requisitos do sistema de gestão financeira pessoal

Nov 10, 2026 · @Ítalo

## 1. Visão geral

Radar é o sistema web de gestão financeira do casal, hospedado em casa, que mostra para onde vai o dinheiro, quanto sobra no mês e quanto falta para cada meta.

**Objetivo.** Concentrar num só lugar rendas, cartões, contas fixas, financiamentos, valores a receber de terceiros e metas de economia, com uma interface simples em estilo dashboard.

**Usuários.** Ítalo e esposa. Os dois têm acesso total a todos os dados.

**Grupos (a partir da Fase 8).** O Radar passa a atender grupos de pessoas cadastradas, como o casal ou amigos que dividem uma casa. Cada pessoa entra com PIN e pode pertencer a vários grupos, e cada grupo vê só os próprios dados. O casal de hoje vira o primeiro grupo. Decisões aprovadas em 12/11/2026; as seções 2.15 e 9 trazem os requisitos e as fases.

**Modelo financeiro.** Caixa único do casal. Todas as rendas formam uma renda única e todos os percentuais são calculados sobre ela.

**Ponto de partida.** O sistema começa do zero, sem importar histórico anterior.

**Escopo da v1.** Contas bancárias, rendas, cartões de crédito, cartões Alelo, contas fixas, financiamentos, compras divididas com terceiros, caixinhas, orçamento por categoria, gráficos, importação de CSV, lista de mercado com comparação de preços entre supermercados, tema claro e escuro, acesso apenas pela rede local.

## 2. Requisitos funcionais

Cada requisito tem um código para ser referenciado nas validações e nos testes.

### 2.1 Usuários e acesso

| ID | Requisito |
| --- | --- |
| RF-01 | Cada pessoa cadastrada entra escolhendo o grupo, depois o próprio nome, e digitando um PIN de 4 a 6 dígitos (RN-27). A tela de entrada lista os nomes dos grupos e, depois de escolhido o grupo, os nomes dos membros dele. Não existe autocadastro: pessoas são criadas pelo administrador do grupo ou pelo terminal (RF-80). |
| RF-02 | Os membros de um grupo veem e editam os dados do grupo. Dados de outro grupo nunca aparecem. |
| RF-03 | Todo lançamento registra qual usuário o criou e quando. |

### 2.2 Contas bancárias

| ID | Requisito |
| --- | --- |
| RF-04 | Cadastrar contas bancárias de cada pessoa com nome, titular e saldo inicial. |
| RF-05 | Calcular o saldo atual de cada conta a partir das entradas e saídas efetivadas. |
| RF-06 | Permitir ajuste manual de saldo, registrado como lançamento de ajuste. |
| RF-07 | Registrar transferências entre contas sem contá-las como gasto ou receita. |
| RF-08 | Indicar de qual conta sai o pagamento de cada fatura e de cada conta fixa. |

### 2.3 Rendas

| ID | Requisito |
| --- | --- |
| RF-09 | Cadastrar rendas recorrentes com valor líquido, dia de recebimento, conta de destino e pessoa. |
| RF-10 | Gerar a cada mês a renda prevista, que só vira efetiva quando o usuário confirmar o recebimento. |
| RF-11 | Cadastrar rendas eventuais previstas, como 13º e férias, com data e valor estimados. |
| RF-12 | Permitir data de término editável em uma renda, como a bolsa, e avisar quando a data se aproximar. |
| RF-13 | Permitir lançar rendas avulsas não previstas. |

### 2.4 Cartões de crédito

| ID | Requisito |
| --- | --- |
| RF-14 | Cadastrar cartões com nome, titular, limite, dia de fechamento, dia de vencimento e aparência visual. |
| RF-15 | Lançar compras à vista com data, valor, estabelecimento, categoria, subcategoria e, opcionalmente, quem fez a compra. |
| RF-16 | Lançar compras parceladas informando valor total ou valor da parcela e número de parcelas, gerando cada parcela na fatura correta. |
| RF-17 | Cadastrar recorrências no cartão, como Spotify, barbearia e internet, lançadas automaticamente a cada fatura. |
| RF-18 | Atribuir cada compra à fatura conforme o dia de fechamento do cartão. |
| RF-19 | Exibir a fatura com total real, cotas de terceiros, valor recebido, valor pendente e parte líquida do casal. |
| RF-20 | Exibir limite usado e limite livre, considerando as parcelas futuras. |
| RF-21 | Registrar o pagamento da fatura, informando a conta de origem. O pagamento é sempre integral, pelo total real da fatura. |
| RF-22 | Lançar estornos e créditos na fatura. |
| RF-23 | Prever as próximas faturas somando parcelas e recorrências já comprometidas. |

### 2.5 Compras divididas com terceiros

| ID | Requisito |
| --- | --- |
| RF-24 | Dividir uma compra, à vista ou parcelada, entre o casal e outras pessoas, registradas apenas pelo nome. |
| RF-25 | Definir a cota de cada pessoa em partes iguais ou em valores personalizados. Nas partes iguais, o usuário escolhe quantas partes o casal conta, começando em uma; a sobra de centavos fica com o casal. |
| RF-26 | Marcar o pagamento de cada pessoa em cada parcela. |
| RF-27 | Sugerir nomes já usados ao digitar, para evitar duplicidade. |
| RF-28 | Listar todas as pendências agrupadas por pessoa, com o total devido. |
| RF-29 | Gerar mensagem de cobrança pronta para enviar pelo WhatsApp, com o detalhamento das parcelas pendentes. |
| RF-30 | Registrar o recebimento em uma conta bancária quando o pagamento for marcado. |

### 2.6 Cartões Alelo

| ID | Requisito |
| --- | --- |
| RF-31 | Cadastrar os dois cartões Alelo com titular, valor do crédito mensal e saldo inicial do dia do cadastro. |
| RF-32 | Lançar o crédito mensal como previsto, confirmado pelo usuário quando cair. |
| RF-33 | Lançar compras apenas com descrição livre e valor, sem categoria. |
| RF-34 | Acumular o saldo não usado para o mês seguinte. |

### 2.7 Contas fixas

| ID | Requisito |
| --- | --- |
| RF-35 | Cadastrar contas fixas fora do cartão com nome, categoria, dia de vencimento, conta de pagamento e valor previsto. |
| RF-36 | Permitir informar o valor real quando a conta chegar, mantendo o previsto para comparação. |
| RF-37 | Marcar cada conta como paga no mês. |
| RF-38 | Exibir calendário de vencimentos com contas fixas e faturas. |

### 2.8 Financiamentos

| ID | Requisito |
| --- | --- |
| RF-39 | Cadastrar financiamentos, como casa, lote, carro e energia solar, com valor da parcela, total de parcelas, parcelas já pagas e saldo devedor. |
| RF-40 | Gerar a parcela mensal como conta a pagar. |
| RF-41 | Exibir parcelas pagas, parcelas restantes e saldo devedor atualizado. |
| RF-42 | Permitir corrigir manualmente o saldo devedor e o valor da parcela, para refletir juros, correção ou amortização. |

### 2.9 Caixinhas

| ID | Requisito |
| --- | --- |
| RF-43 | Criar caixinhas com nome, valor-alvo e percentual mensal sobre todas as entradas do mês. |
| RF-44 | Editar valor-alvo e percentual a qualquer momento. |
| RF-45 | Sugerir o aporte do mês de cada caixinha com base no percentual. |
| RF-46 | Registrar o aporte somente após confirmação do usuário, com o valor efetivamente depositado, que sai de uma conta bancária escolhida. A retirada volta para uma conta. Nenhum dos dois conta como gasto (RN-08). |
| RF-47 | Permitir retiradas, registradas com valor e motivo. |
| RF-48 | Exibir saldo, progresso e data estimada de conclusão de cada caixinha. |
| RF-49 | Concluir a caixinha ao atingir o valor-alvo. |

### 2.10 Orçamento e categorias

| ID | Requisito |
| --- | --- |
| RF-50 | Manter categorias e subcategorias, permitindo criar, renomear, mover e desativar. |
| RF-51 | Definir orçamento mensal por categoria principal, somando todas as subcategorias dela. |
| RF-52 | Alertar quando o gasto de uma categoria atingir 80% e 100% do orçamento. |

### 2.11 Importação de CSV

| ID | Requisito |
| --- | --- |
| RF-53 | Importar o CSV de fatura do Nubank, com pré-visualização antes de gravar. |
| RF-54 | Manter regras de apelido que trocam o nome original do estabelecimento por um nome amigável. |
| RF-55 | Aplicar automaticamente as regras de apelido e de categoria nas importações seguintes. |
| RF-56 | Detectar lançamentos possivelmente duplicados e pedir confirmação antes de gravar. |
| RF-57 | Reconhecer parcelas no CSV e vinculá-las à compra parcelada correspondente. Quando a compra não existe no Radar, ela é criada só da parcela do CSV em diante, sem alterar faturas passadas. |

### 2.12 Relatórios e indicadores

| ID | Requisito |
| --- | --- |
| RF-58 | Calcular o saldo previsto do mês: renda menos todas as saídas previstas e aportes nas caixinhas. A fatura do cartão entra pela parte do casal, sem as cotas de terceiros (RN-04). |
| RF-59 | Exibir gráfico de evolução dos gastos mês a mês, total e por categoria. |
| RF-60 | Exibir o comprometimento da renda futura com parcelas e financiamentos. |
| RF-61 | Gerar fechamento do mês com entradas, saídas, valor guardado e valores a receber. |
| RF-62 | Exibir patrimônio líquido: saldos em conta e caixinhas menos saldos devedores. Dos cartões, entram como dívida só as faturas já fechadas e não pagas; parcelas futuras ficam no comprometimento da renda (RF-60). |
| RF-63 | Filtrar lançamentos por período, categoria, cartão, conta, pessoa e texto. |

### 2.13 Notificações

| ID | Requisito |
| --- | --- |
| RF-64 | Exibir avisos dentro do sistema para vencimentos próximos, contas atrasadas, orçamento estourado, aportes pendentes de confirmação e fim de renda temporária. |

### 2.14 Lista de mercado e comparação de preços

Módulo em tela própria para descobrir onde a compra do mês sai mais barata, considerando o total da lista e não item a item.

| ID | Requisito |
| --- | --- |
| RF-65 | Cadastrar mercados com nome e bairro. |
| RF-66 | Cadastrar produtos sempre de marca específica, com nome, marca, tamanho e unidade, como Café Três Corações 500 g. Não existe produto genérico. Itens vendidos por peso, como hortifrúti e carnes, são produtos com unidade kg e marca livre ("Sem marca" ou a do mercado): o preço é por kg e a quantidade na lista aceita decimais, como 1,5 kg. |
| RF-67 | Registrar preços por produto e mercado, com data, mantendo o histórico. |
| RF-68 | Importar preços escaneando o QR code da NFC-e com o celular: o sistema consulta a nota na Sefaz e registra todos os itens com o preço daquele mercado e daquela data. |
| RF-69 | Vincular o nome abreviado do item na nota ao produto cadastrado, por mercado, e reconhecer automaticamente nas notas seguintes. |
| RF-70 | Permitir cadastro e correção manual de preços, como alternativa à nota fiscal. |
| RF-71 | Montar listas de compras com produtos e quantidades, compartilhadas entre os dois usuários em tempo real, com marcação de item pego. |
| RF-72 | Calcular o total da lista em cada mercado e exibir o ranking do mais barato ao mais caro. |
| RF-73 | Calcular a melhor divisão da compra entre dois ou mais mercados, indicando o que comprar em cada um e a economia em relação ao melhor mercado único. |
| RF-74 | Permitir configurar o número máximo de mercados por compra e a economia mínima para valer a pena incluir mais um mercado. |
| RF-75 | Exibir aviso de preço desatualizado e de itens sem preço em cada mercado. |
| RF-76 | Exibir o histórico de preço de cada produto por mercado. |
| RF-77 | Finalizar a compra gerando o lançamento do gasto no Alelo, no crédito ou em dinheiro, com o total real pago. |

### 2.15 Grupos

| ID | Requisito |
| --- | --- |
| RF-78 | Cadastrar grupos com nome. Uma pessoa pode pertencer a vários grupos. |
| RF-79 | Cada membro tem papel, administrador ou membro, e peso no grupo, usado no acerto (RF-84). |
| RF-80 | O administrador do grupo renomeia o grupo, cria outros grupos, cadastra pessoas novas com login e PIN e define papéis. Para adicionar uma pessoa já cadastrada, a própria pessoa digita o PIN dela, como consentimento; login ou PIN errado dão a mesma resposta. O administrador só redefine o PIN de quem pertence apenas a grupos que ele administra; os demais PINs, só pelo terminal. O primeiro grupo e o primeiro administrador são criados pelo terminal. Regras de consentimento e de PIN decididas em 12/11/2026. |
| RF-81 | Quem está em vários grupos troca o grupo ativo por um seletor, sem entrar de novo. Todas as telas mostram só os dados do grupo ativo. |
| RF-82 | Contas bancárias, cartões de crédito e cartões Alelo pertencem à pessoa e aparecem nos grupos de que ela faz parte. |
| RF-83 | Cada lançamento, compra no cartão, conta fixa, financiamento, renda e caixinha é pessoal ou de um grupo. O pessoal é visível só para quem o criou. Cada pessoa tem um espaço Pessoal, no topo do seletor de grupo, com visão geral e relatórios só dos itens pessoais; as telas de um grupo mostram só o que é do grupo. Ao lançar, vale o espaço ativo, e o formulário permite escolher o Pessoal ou outro grupo da pessoa; a edição move o item, com os lançamentos dele, entre esses espaços, exceto compra dividida com terceiros. A dona do cartão vê no Pessoal a fatura completa, com o grupo de cada linha, e paga a fatura inteira de uma conta dela; cada grupo vê só a parte dele, sem pagar. O saldo real da conta, com todos os movimentos, aparece só para a dona, e o extrato completo fica no Pessoal dela; o patrimônio do grupo não soma contas, o do Pessoal soma. A previsão do mês de cada espaço conta só a parte dele da fatura. O Alelo segue a mesma regra da conta: o saldo real e os créditos aparecem só para quem é titular, o grupo vê as compras feitas nele e o extrato completo fica no Pessoal de quem é titular. Recorrências do cartão e compras do Alelo também têm o campo Onde ao lançar; a compra do Alelo pode ser movida, mas a recorrência não, para não reescrever faturas fechadas: para trocar o espaço, encerra-se a recorrência e cria-se outra. Decidido em 13/11/2026. |
| RF-84 | Exibir o saldo de acerto de cada membro do grupo e registrar o pagamento entre membros. Entram no acerto os gastos de conta do grupo (inclusive pagamentos de contas fixas e parcelas de financiamento), as compras e recorrências no cartão e as compras no Alelo; quem pagou é quem tem a conta, o cartão ou o Alelo. Caixinhas, rendas, transferências e recebimentos de terceiros ficam de fora. Cada parcela do cartão entra quando a fatura dela fecha. Numa compra dividida com terceiros, os membros dividem só a parte do grupo, e o que os terceiros devolvem fica com quem pagou. Ao lançar, a divisão entre membros é pelos pesos, em partes iguais entre quem participou ou por valor de cada membro. O Radar sugere quem paga quem com o menor número de transferências. O pagamento de acerto é registrado ou desfeito só por quem paga, quem recebe ou um administrador do grupo, com data entre o início do acerto e hoje. O titular de uma conta, cartão ou Alelo que pagou gastos de um grupo não muda, para o acerto não trocar quem pagou. Decidido em 13/11/2026. |
| RF-85 | Mercados, produtos, preços, notas fiscais e listas de compras são de cada grupo. |

## 3. Regras de negócio

As regras abaixo definem como o sistema calcula e classifica os valores. Elas viram casos de teste na fase de validação.

| ID | Regra |
| --- | --- |
| RN-01 | Compras no cartão pertencem ao mês da fatura. A data da compra é guardada apenas como informação. O mês da fatura é o mês do vencimento: a fatura que fecha em 28/11 e vence em 05/12 é a de dezembro. |
| RN-02 | Compra feita no dia de fechamento ou depois entra na fatura seguinte. |
| RN-03 | Em compra parcelada, cada parcela cai em uma fatura consecutiva a partir da primeira. Diferenças de centavos na divisão vão para a primeira parcela. Numa compra dividida com outras pessoas, a cota de cada pessoa acompanha o valor de cada parcela, com pelo menos um centavo em cada uma; os centavos que sobram das cotas vão para as parcelas com maior resto, e as cotas de uma parcela nunca somam mais que ela; assim a parte do casal de um mês nunca fica negativa. Vale para compras lançadas ou editadas a partir de 12/11/2026. |
| RN-04 | Nas análises e gráficos de gasto, só a cota do casal conta como gasto. A cota de terceiros é valor a receber. |
| RN-05 | Parte líquida da fatura = total real da fatura menos a soma das cotas de terceiros. |
| RN-06 | A cota de um terceiro sai de pendente quando marcada como paga, gerando entrada na conta escolhida. |
| RN-07 | Pagamento de fatura não é gasto novo. Ele só reduz o saldo da conta, pois as compras já foram contadas. |
| RN-08 | Transferências entre contas e aportes nas caixinhas não contam como gasto. |
| RN-09 | A base de cálculo das caixinhas é a soma de todas as entradas efetivas do mês: salários, bolsa, 13º, férias e rendas avulsas. Cada entrada conta no mês da data em que caiu na conta. Pagamentos de terceiros e crédito do Alelo não entram na base. |
| RN-10 | Aporte sugerido = percentual da caixinha × base do mês, limitado ao que falta para o valor-alvo. |
| RN-11 | Data estimada de conclusão usa a média dos aportes confirmados nos últimos três meses. |
| RN-12 | Uma caixinha concluída para de receber sugestões de aporte, mas continua visível no histórico. |
| RN-13 | Renda prevista entra no saldo previsto, mas só altera o saldo da conta quando confirmada. |
| RN-14 | Conta fixa usa o valor previsto até que o valor real seja informado. |
| RN-15 | Saldo do Alelo = saldo do mês anterior + crédito confirmado − compras do mês. Gastos do Alelo entram no total de gastos como Alimentação, compras para casa. |
| RN-16 | Saldo devedor de financiamento diminui a cada parcela paga, conforme o valor informado pelo usuário: ao pagar, ele informa o novo saldo devedor, sugerido como o saldo atual menos a parcela. |
| RN-17 | Limite livre do cartão = limite total menos todas as parcelas e compras ainda não pagas. |
| RN-18 | Categoria desativada some das opções de lançamento, mas mantém o histórico. Uma categoria com lançamentos não pode ser excluída. |
| RN-19 | Regra de apelido casa pelo texto original do estabelecimento no CSV. A regra mais recente prevalece em caso de conflito. |
| RN-20 | Lançamento é considerado possível duplicata quando cartão, valor e data coincidem, com tolerância de um dia na data. |
| RN-21 | Valores monetários são guardados em centavos inteiros, sem ponto flutuante. Preço unitário com mais de duas casas, como R$ 6,499 na nota fiscal, é truncado em centavos: vira R$ 6,49. |
| RN-22 | A comparação entre mercados usa o preço mais recente de cada produto em cada mercado. Produtos de marcas ou tamanhos diferentes nunca são tratados como equivalentes. |
| RN-23 | Total da lista num mercado = soma de preço × quantidade dos itens com preço naquele mercado. O mercado continua no ranking quando faltam preços, com aviso de quantos itens faltam. O ranking mostra primeiro os mercados com preço de todos os itens, do mais barato ao mais caro, e depois os incompletos, do que falta menos item para o que falta mais e pelo total; mercado sem nenhum preço da lista fica fora. Item por peso custa preço por kg × quantidade, truncado em centavos. |
| RN-24 | Melhor divisão: para cada combinação de mercados até o máximo configurado, cada item vai para o mercado mais barato da combinação. Vence a combinação de menor total que cubra todos os itens, desde que cada mercado a mais economize pelo menos o valor mínimo configurado. A comparação é passo a passo: a melhor combinação com um mercado a mais só vence se economizar o mínimo em relação à melhor combinação com um a menos; quando não economiza, a busca para. Item sem preço em nenhum mercado fica fora da divisão, com aviso. |
| RN-25 | Preço com mais de 30 dias sem atualização é marcado como desatualizado. O prazo é configurável. |
| RN-26 | A compra finalizada entra nos gastos como Alimentação, Compras para casa, exceto quando paga com Alelo, que segue a RN-15. |
| RN-27 | O PIN tem de 4 a 6 dígitos e é guardado só como hash. Cinco erros seguidos bloqueiam a entrada daquela pessoa por 15 minutos. |
| RN-28 | A cota de cada membro numa despesa do grupo segue os pesos do grupo, com a regra de centavos da RN-03; a despesa pode usar outra divisão na hora de lançar. Só entra no acerto o que é do grupo. Vale o peso e a participação de cada membro na data da despesa (no fechamento da fatura, para o cartão) e, no próprio dia de uma mudança, só para o que foi lançado depois dela; então mudar um peso ou entrar no grupo não muda despesas anteriores. Mudar a divisão de um item também vale daqui para frente: pagamentos e faturas já fechadas mantêm a divisão que tinham. Os centavos que sobram vão para os membros com maior resto e, no empate, para quem entrou antes no grupo. |
| RN-29 | Saldo de acerto de um membro = o que ele pagou de despesas do grupo, menos a cota dele nessas despesas, mais os pagamentos de acerto que fez, menos os que recebeu. A soma dos saldos do grupo é sempre zero. |
| RN-30 | O pagamento de acerto entre membros é uma transferência entre contas de pessoas diferentes e não conta como gasto nem como renda. |
| RN-31 | No grupo criado com os dados do casal, o acerto começa zerado na data da migração; nada anterior entra no saldo. Grupos novos começam o acerto na data em que são criados. Parcelas de compras no cartão feitas antes do início do acerto não entram, mesmo que a fatura feche depois; recorrências entram pelas faturas que fecham a partir do início. |

## 4. Requisitos não funcionais

O sistema roda em Docker na máquina pessoal, acessível só pela rede local, e precisa sobreviver a desligamentos sem perder dados.

| ID | Área | Requisito |
| --- | --- | --- |
| RNF-01 | Infraestrutura | Todos os serviços rodam em contêineres orquestrados por Docker Compose. |
| RNF-02 | Infraestrutura | Os contêineres sobem automaticamente quando a máquina liga. |
| RNF-03 | Persistência | O banco de dados fica em volume Docker, preservado em desligamentos, reinícios e atualizações. |
| RNF-04 | Persistência | Toda gravação é transacional, sem perda de dados em desligamento inesperado. |
| RNF-05 | Backup | Backup automático diário do banco em pasta fora do volume, mantendo as últimas 30 cópias. |
| RNF-06 | Backup | Backup ao ligar a máquina quando o backup do dia anterior não tiver rodado. |
| RNF-07 | Backup | Botão para gerar backup manual e procedimento documentado de restauração. |
| RNF-08 | Rede | Acesso apenas pela rede local, sem portas abertas no roteador. |
| RNF-09 | Segurança | Sem senhas por decisão do casal: a proteção é o acesso só pela rede local (RNF-08). Antes de qualquer acesso externo (RNF-11), avaliar um PIN de entrada. |
| RNF-10 | Segurança | Sessão expira após período de inatividade configurável. |
| RNF-11 | Segurança | Arquitetura preparada para acesso externo futuro via VPN, sem mudanças no modelo de dados. |
| RNF-12 | Usabilidade | Interface responsiva, usável no celular e no computador. |
| RNF-13 | Usabilidade | Lançamento rápido de uma compra em no máximo quatro toques no celular. |
| RNF-14 | Usabilidade | Tema claro e escuro desde a primeira versão. |
| RNF-15 | Usabilidade | Modo privacidade que oculta todos os valores com um clique. |
| RNF-16 | Usabilidade | Interface em português do Brasil, valores em reais e datas no formato dia, mês e ano. |
| RNF-17 | Desempenho | Dashboard carrega em menos de dois segundos na rede local. |
| RNF-18 | Dados | Exportação completa dos dados em CSV. |
| RNF-19 | Manutenção | Atualização do sistema por um único comando, com migrações de banco automáticas. |
| RNF-20 | Plataforma | As plataformas móveis suportadas são o iPhone com Safari e o Android com Chrome, as duas com testes de ponta a ponta. Android incluído em 13/11/2026. |
| RNF-21 | Rede | Acesso por HTTPS na rede local, com Caddy como proxy e certificado próprio. O certificado raiz é instalado uma vez em cada celular, iPhone ou Android. Requisito para o PWA funcionar. No Android, o endereço usado é o IP reservado no roteador, que o certificado também cobre, porque o Chrome do Android não resolve nomes `.local` de forma confiável. |
| RNF-22 | Rede | Endereço fixo para o servidor, por IP reservado no roteador ou nome local como financas.local. |
| RNF-23 | Usabilidade | PWA instalável na tela inicial do celular, pelo Safari no iPhone e pelo Chrome no Android, abrindo em tela cheia e respeitando a área segura do indicador de início, do entalhe e das barras do sistema. No Android, a cor da barra do sistema acompanha o tema claro ou escuro. |
| RNF-24 | Usabilidade | Gestos de pressionar e segurar bloqueiam a seleção de texto e o menu de contexto do iOS e do Android. |
| RNF-25 | Usabilidade | Resposta tátil opcional, no iPhone pelo contorno do interruptor do iOS 18 e no Android pela vibração do navegador, desativável e sem nenhuma função dependente dela. |
| RNF-26 | Usabilidade | No Android, o botão e o gesto de voltar fecham primeiro a folha, o diálogo ou o menu aberto, sem sair da tela; só depois voltam à tela anterior. |

## 5. Interface

A interface segue a referência enviada: menu lateral fixo, navegação por mês no topo e cards brancos com números grandes, centavos menores e ilustrações 3D.

### 5.1 Elementos globais

- Menu lateral com as telas e o usuário logado no rodapé;
- Seletor de mês com setas no topo, valendo para todas as telas;
- Botão Adicionar com atalhos para compra no cartão, despesa, receita, compra no Alelo e importação de CSV;
- Sino de notificações com contador;
- Alternador de tema claro e escuro e botão de modo privacidade.

### 5.2 Telas

| Tela | Conteúdo |
| --- | --- |
| Visão geral | Dashboard com os cards da seção 5.3. |
| Fluxo de caixa | Entradas e saídas do mês, saldos das contas e gráfico de evolução dos gastos. |
| Cartões | Carrossel com o visual de cada cartão, fatura detalhada, parcelas, recorrências e compras divididas. |
| Alelo | Saldo acumulado e compras do mês de cada cartão. |
| Contas fixas e financiamentos | Cadastro, calendário de vencimentos, parcelas restantes e saldo devedor. |
| Caixinhas | Metas, progresso, aportes sugeridos, confirmação de depósito e histórico. |
| A receber | Pendências por pessoa, marcação de pago e mensagem de cobrança. |
| Ajustes | Contas bancárias, rendas, categorias, orçamentos, regras de apelido, usuários e backup. |
| Mercado | Listas de compras, ranking de mercados, melhor divisão da compra, leitura de nota fiscal, produtos e histórico de preços. |

### 5.3 Cards da visão geral

| Card | O que mostra |
| --- | --- |
| Saldo previsto | Valor que sobra no mês e barra empilhada da renda até a sobra, com valor e percentual de cada grupo. |
| Contas a pagar | Total do mês, quantas foram pagas, chips com os dias de vencimento e lista com marcação de pago. |
| Cartões | Visual do cartão, fatura do mês com total real e parte líquida, limite livre e gráfico das próximas faturas. |
| Valores a receber | Total pendente de terceiros e próximas cobranças, com marcação de pago. |
| Moradia | Percentual da renda do casal destinado à casa, com régua de referência. |
| Caixinhas | Progresso de cada meta e aportes do mês aguardando confirmação. |
| Financiamentos | Parcelas restantes e saldo devedor de cada financiamento. |
| Evolução dos gastos | Gráfico dos últimos meses por categoria. |
| Orçamento | Categorias perto ou acima do limite. |

### 5.4 Visual dos cartões

Cada cartão é desenhado como um cartão físico, com cor e acabamento configuráveis, nome, titular, final do número, vencimento e limite livre. O cartão principal usa acabamento escuro e metálico. Entre as cores há a opção Nubank Croma, que reproduz a face do cartão real: cinza cromado com granulado, logo roxa, marca Mastercard e chip só em contorno. Os cartões Alelo podem usar o visual Alelo Tudo, vertical, verde-limão com a linha que passa pelo chip e vira a haste do "d" de "tudo", ou o visual clássico verde.

### 5.5 Navegação

O menu segue a mesma linguagem visual nos três estados: indicadores que deslizam com efeito de mola, feitos com o layout compartilhado do Motion.

| Estado | Comportamento |
| --- | --- |
| Computador, menu aberto | Padrão. Ícone e nome de cada tela. O destaque do item ativo desliza até o item clicado e um destaque suave acompanha o mouse entre os itens. |
| Computador, menu recolhido | Só ícones. Tooltip fluido que desliza e se transforma de um ícone para o outro conforme o mouse passa, inspirado no [Fluid Tooltip Animation](https://www.inspora.design/posts/fluid-tooltip-animation). |
| Celular (iPhone e Android) | Barra inferior com cinco itens: Visão geral, Cartões, Adicionar em destaque no centro, Caixinhas e Mais. Pressionar e segurar expande os rótulos de todos os itens, inspirado no [Playful Bottom Nav](https://www.inspora.design/posts/playful-bottom-nav). O rótulo do item ativo fica sempre visível. |

O item Mais abre as telas restantes: Fluxo de caixa, Alelo, Mercado, Contas fixas e financiamentos, A receber e Ajustes.

### 5.6 Lançamento rápido de gasto

Card único aberto pelo botão Adicionar, inspirado no card de valor com rolagem de dígitos da referência em vídeo. Os blocos aparecem de cima para baixo:

1. **Natureza** → chave entre Gasto e Receita no topo do card;
2. **Forma de pagamento** → seletor segmentado entre Crédito, Dinheiro e Alelo, exibido só para gastos. Dinheiro cobre tudo que sai direto da conta, como Pix, débito e boleto;
3. **Tipo** → seletor segmentado entre Fixo e Variável, oculto quando a forma de pagamento é Alelo;
4. **Valor** → número grande no centro, digitado pelo teclado numérico do celular e com rolagem por dígito a cada tecla. Sem botões de mais e menos;
5. **Categoria** → grade de ícones das categorias principais. Tocar num ícone expande as subcategorias dela logo abaixo, com animação de layout, e tocar numa subcategoria conclui a escolha. Oculta para Alelo e para receitas;
6. **Detalhes** → campos que mudam conforme as escolhas acima, na tabela seguinte;
7. **Confirmar** → botão com o check animado.

| Combinação | Vira no sistema | Campos extras |
| --- | --- | --- |
| Gasto, Crédito e Variável | Compra no cartão | Cartão, número de parcelas com botões de mais e menos, estabelecimento, quem fez, dividir com terceiros |
| Gasto, Crédito e Fixo | Recorrência no cartão | Cartão, descrição, data de início |
| Gasto, Dinheiro e Variável | Saída avulsa da conta | Conta de origem, descrição |
| Gasto, Dinheiro e Fixo | Conta fixa | Conta de pagamento, dia de vencimento, valor previsto |
| Gasto e Alelo | Compra no Alelo | Cartão Alelo, descrição livre |
| Receita e Variável | Renda avulsa | Conta de destino, descrição |
| Receita e Fixo | Renda recorrente | Conta de destino, dia de recebimento, pessoa, data de término opcional |

A data vem preenchida com hoje e o último cartão ou conta usado vem pré-selecionado, para cumprir o lançamento em poucos toques do RNF-13. Por decisão do dono, um toque no botão Adicionar abre o card já em Gasto, Crédito e Variável; os outros atalhos ficam no gesto de pressionar e segurar.

Combinações que dependem de módulos ainda não construídos ficam ocultas até a fase deles: Conta fixa e Alelo até a Fase 3, dividir com terceiros até a Fase 4.

**Ícones das categorias**

| Categoria | Ícone sugerido |
| --- | --- |
| Moradia | Casa |
| Alimentação | Talheres |
| Carro | Carro |
| Transporte | Ônibus |
| Saúde | Coração com pulso |
| Cuidados pessoais | Tesoura |
| Assinaturas | Repetir |
| Lazer | Ingresso |
| Compras | Sacola |
| Educação | Capelo |
| Pets | Pegada |
| Financeiro | Banco |
| Outros | Reticências |

Categorias criadas pelo usuário escolhem um ícone numa biblioteca, como Lucide.

## 6. Categorias iniciais

Lista carregada na instalação. Todas podem ser renomeadas, movidas ou desativadas, e novas podem ser criadas a qualquer momento.

| Categoria | Subcategorias |
| --- | --- |
| Moradia | Financiamento da casa, Lote, Energia, Energia solar, Água, Internet, Manutenção e reparos, Móveis e utilidades |
| Alimentação | Compras para casa, Delivery, Restaurantes e lanches |
| Carro | Financiamento, Combustível, Mecânico, Seguro, IPVA e licenciamento, Estacionamento e pedágio, Lavagem |
| Transporte | Aplicativos de corrida, Transporte público |
| Saúde | Farmácia, Consultas e exames, Plano de saúde, Academia |
| Cuidados pessoais | Barbearia e salão, Cosméticos e higiene |
| Assinaturas | Streaming, Software e serviços digitais, Telefonia |
| Lazer | Passeios e eventos, Viagens, Hobbies |
| Compras | Roupas e calçados, Eletrônicos, Presentes |
| Educação | Cursos, Livros e material |
| Pets | Ração, Veterinário, Petshop |
| Financeiro | Tarifas bancárias, Juros e multas, Impostos |
| Outros | Sem categoria definida |

## 7. Modelo de dados conceitual

O centro do modelo é o Lançamento: toda movimentação de dinheiro, prevista ou efetiva, é um lançamento ligado a uma conta, a um cartão ou a uma caixinha.

| Entidade | Principais atributos | Relaciona-se com |
| --- | --- | --- |
| Usuário | nome, login, PIN, preferência de tema | cria Lançamentos |
| Pessoa | nome | membro de Grupos; titular de Conta, Cartão, Alelo e Renda; autora opcional de Compra |
| Grupo | nome, tipo (compartilhado ou pessoal) | tem Membros; possui os dados do grupo; o pessoal tem um só membro e guarda os itens pessoais dele |
| Membro | papel, peso | liga Pessoa e Grupo |
| Acerto | data, valor, de quem, para quem | pertence a um Grupo; é uma Transferência entre contas de Membros |
| Conta bancária | nome, titular, saldo inicial | recebe e paga Lançamentos |
| Renda | tipo, valor líquido, dia, data de término, recorrente ou eventual | gera Lançamentos previstos em uma Conta |
| Cartão de crédito | nome, titular, limite, fechamento, vencimento, visual | possui Faturas |
| Fatura | mês de referência, status, data de pagamento, conta de pagamento | agrupa Parcelas e Recorrências |
| Compra | data, estabelecimento, valor total, número de parcelas, categoria, autor | gera Parcelas; pode ter Divisões |
| Parcela | número, valor, fatura | pertence a uma Compra e a uma Fatura |
| Recorrência | descrição, valor, categoria, início, fim | gera Parcelas mensais no Cartão |
| Divisão | nome do terceiro, cota | pertence a uma Compra; tem Pagamentos por parcela |
| Pagamento de terceiro | parcela, pago, data, conta de destino | pertence a uma Divisão |
| Cartão Alelo | titular, crédito mensal | possui Compras Alelo e Créditos |
| Compra Alelo | data, descrição livre, valor | pertence a um Cartão Alelo |
| Conta fixa | nome, categoria, vencimento, valor previsto, conta de pagamento | gera Lançamentos mensais |
| Financiamento | nome, parcela, total de parcelas, parcelas pagas, saldo devedor | gera Lançamentos mensais |
| Caixinha | nome, valor-alvo, percentual, status | recebe Aportes e Retiradas |
| Aporte | mês, valor sugerido, valor confirmado, data | pertence a uma Caixinha |
| Categoria | nome, categoria mãe, ativa, orçamento mensal | classifica Compras, Contas fixas e Lançamentos |
| Regra de apelido | texto original, nome amigável, categoria padrão | aplicada na Importação |
| Importação | arquivo, data, cartão, itens importados | gera Compras |
| Lançamento | data, valor em centavos, tipo, previsto ou efetivo, conta, categoria, origem | liga todas as movimentações |
| Notificação | tipo, mensagem, lida, referência | aponta para qualquer entidade |
| Mercado | nome, bairro | possui Preços e Notas fiscais |
| Produto | nome, marca, tamanho, unidade | tem Preços por Mercado; aparece em Itens de lista |
| Preço | valor em centavos, data, origem nota ou manual | liga Produto e Mercado; pode vir de uma Nota fiscal |
| Nota fiscal | chave de acesso, data, total | pertence a um Mercado; gera Preços |
| Vínculo de item da nota | texto ou código do item na nota | liga Mercado e Produto |
| Lista de compras | nome, status, data | possui Itens de lista; gera Lançamento ao finalizar |
| Item de lista | quantidade, pego | liga Lista e Produto |

## 8. Arquitetura e stack

Backend em Go com SQLite e frontend em React, entregues como um único binário num único contêiner Docker.

### 8.1 Backend

| Camada | Escolha | Motivo |
| --- | --- | --- |
| Linguagem | Go | Linguagem do dia a dia do desenvolvedor; concentra todas as regras de cálculo. |
| HTTP | net/http da biblioteca padrão, com chi se necessário | Roteamento por método e parâmetro sem dependências extras. |
| Banco | SQLite em modo WAL, driver modernc.org/sqlite | Arquivo único, backup simples, sem CGO. |
| Consultas | sqlc | Código Go tipado gerado a partir de SQL escrito à mão. |
| Migrações | goose | Executadas automaticamente na inicialização. |
| Autenticação | scs para sessões, sem senhas | Simples para dois usuários; a sessão identifica quem fez cada lançamento. |
| Contrato | OpenAPI com oapi-codegen | Especificação única que gera o servidor Go e os tipos do front. |
| Backup | VACUUM INTO agendado no próprio serviço | Cópia consistente com o sistema em uso e recuperação do backup perdido ao ligar a máquina. |
| Dinheiro | int64 em centavos | Sem ponto flutuante do banco até a API. |
| Testes | go test com testes em tabela | Um caso de teste por regra de negócio. |

### 8.2 Frontend

| Camada | Escolha | Motivo |
| --- | --- | --- |
| Base | React com TypeScript e Vite | Ecossistema mais rico de componentes visuais. |
| Estilo | Tailwind CSS com shadcn/ui | Visual da referência com tema claro e escuro nativos. |
| Animações | Motion | Animações declarativas, de layout e por gestos. NumberFlow para a rolagem de dígitos dos valores, com formatação em reais. |
| Gráficos | Recharts | Linhas, barras empilhadas e gráficos de área com animação de entrada. |
| Dados | TanStack Query | Cache e atualização automática das telas após cada lançamento. |
| Formulários | React Hook Form com Zod | Validação no cliente, inclusive na pré-visualização do CSV. |
| Tipos da API | openapi-typescript | Tipos gerados da mesma especificação do backend. |
| Celular | PWA | Instalável como um app na tela inicial, pelo Safari do iPhone e pelo Chrome do Android. Exige HTTPS. |
| Testes | Playwright | Fluxos de ponta a ponta das telas principais. |

### 8.3 Implantação

- O build do React é embutido no binário Go com embed;
- Imagem Docker com um único processo e política de reinício automático;
- Volume para o banco e pasta separada para os backups;
- Docker Compose como único ponto de configuração.
- Caddy no Docker Compose como proxy HTTPS com certificado próprio para a rede local;
- IP reservado no roteador ou nome local para o servidor.

### 8.4 Ambiente de desenvolvimento

Na máquina ficam só Docker, VS Code com a extensão Dev Containers, Git e WSL2. Todo o resto vive num Dev Container descrito no repositório.

| Onde | O que tem |
| --- | --- |
| Máquina | Docker Desktop com backend WSL2, VS Code, extensão Dev Containers, Git |
| Dev Container, backend | Go, sqlc, goose, oapi-codegen, golangci-lint |
| Dev Container, frontend | Node LTS com pnpm |
| Dev Container, testes | Playwright com WebKit para simular o Safari do iPhone e Chromium para simular o Chrome do Android |
| Dev Container, utilitários | sqlite3, mkcert |
| Dev Container, VS Code | Extensões de Go, ESLint, Tailwind CSS e Playwright instaladas automaticamente |

O repositório fica clonado dentro do sistema de arquivos do WSL2, nunca em C:, para manter o desempenho. O mesmo Dev Container funciona na máquina Linux.

### 8.5 Servidor em casa no Windows

A produção roda na máquina Windows, que é a que fica mais tempo ligada. Isso traz requisitos de configuração do próprio Windows.

| Ponto | Configuração |
| --- | --- |
| Início automático | Docker Desktop configurado para iniciar com o Windows. Como ele só sobe após o login, o Windows precisa de login automático ou o sistema fica fora do ar depois de um reinício |
| Suspensão | Plano de energia sem suspensão nem hibernação enquanto ligado na tomada |
| Windows Update | Horário ativo configurado para que reinícios aconteçam de madrugada |
| Rede | Perfil da rede de casa como Privada e regra no Firewall do Windows liberando a porta 443 só para redes privadas |
| Endereço | IP reservado no roteador para a máquina. O certificado inclui esse IP e o nome do computador |
| Dados | Banco no volume Docker dentro do WSL2. Backups gravados numa pasta do Windows, de preferência em outro disco, para sobreviverem a um problema no WSL2 |
| Verificação | Página de saúde do sistema mostrando data do último backup e tempo ligado |

### 8.6 Animações propostas

Animações curtas, entre 150 e 400 milissegundos, que dão retorno a uma ação ou guiam o olhar, sem atrasar o uso.

| Onde | Animação |
| --- | --- |
| Valores grandes | Rolagem por dígito no estilo odômetro: só os dígitos que mudam se movem, passando pelos valores intermediários em cerca de 350 ms. Ao aumentar, o dígito novo entra por baixo e o antigo sai por cima; ao diminuir, o contrário. Dígitos em movimento ficam esmaecidos e assentam com leve efeito de mola. O texto auxiliar abaixo do valor troca com fade. Vale para saldo previsto, fatura, totais ao trocar de mês e valores a receber. |
| Entrada do dashboard | Entrada em duas ondas, em cerca de um segundo. Primeiro as cascas dos cards surgem em sequência, saindo de levemente menores, transparentes e desfocadas até o tamanho e a nitidez finais. Depois o conteúdo de cada card aparece com fade, e destaques como o dia atual do calendário entram por último com um pequeno salto. |
| Troca de mês | Conteúdo desliza para o lado da seta clicada. |
| Barras e progresso | Barras com preenchimento em gradiente de verde, do tom mais escuro ao mais claro, que se preenchem da esquerda para a direita e deslizam suavemente até o novo valor quando ele muda, com o percentual atualizando junto. Vale para saldo previsto, orçamentos e progresso das caixinhas. O percentual mensal de cada caixinha é ajustado num controle deslizante com o mesmo visual e um pino branco redondo. |
| Gráficos | Linhas se desenham e barras crescem a partir da base. |
| Cartões | Inclinação 3D seguindo o mouse, com reflexo de luz no acabamento metálico, e carrossel com arraste no celular. |
| Marcar como pago | Check desenhado no círculo e item esmaecendo para o fim da lista. |
| Aporte confirmado | Moeda caindo na caixinha e barra de progresso avançando. |
| Caixinha concluída | Confete ao atingir o valor-alvo. |
| Terceiro quitou | Nome riscado e contador de pendências diminuindo. |
| Modo privacidade | Valores desfocam suavemente. |
| Troca de tema | Transição circular a partir do botão, com a View Transitions API. |
| Botão Adicionar | Um toque abre direto o lançamento de compra no cartão. Os atalhos abrem em leque ao pressionar e segurar no celular, e pela seta ao lado do botão no computador. |

Todas as animações respeitam a preferência de movimento reduzido do sistema operacional.

## 9. Fases de construção

O Radar é construído em fases, cada uma usada e validada antes da próxima. As fases 8 a 11, aprovadas em 12/11/2026, levam o Radar do casal para grupos. A fase 12, incluída em 13/11/2026, leva o Radar ao Android. Em toda fase, os testes das regras de negócio envolvidas são escritos antes do código, e as animações da seção 8.6 entram junto com os componentes que elas animam.

| Fase | Entrega | Requisitos | Pronta quando |
| --- | --- | --- | --- |
| 1. Fundação | Dev Container, repositório, Go com SQLite e migrações, contrato OpenAPI, login, casca do React com menu lateral, barra do iPhone e tema, Docker Compose com Caddy, backup e página de saúde. Prova de conceito isolada da consulta de NFC-e na Sefaz de Goiás | RF-01 a RF-03, RNF-01 a RNF-25, seção 5.5 | Os dois iPhones abrem o Radar por HTTPS como app, fazem login e o backup diário aparece na pasta do Windows |
| 2. Núcleo | Contas bancárias, rendas, categorias, lançamento rápido, cartões com faturas, parcelas e recorrências | RF-04 a RF-23, RF-50, seção 5.6, RN-01 a RN-03, RN-07, RN-08, RN-13, RN-17, RN-18, RN-21 | A fatura do mês no Radar bate com a fatura real do Nubank |
| 3. Compromissos | Contas fixas, financiamentos, calendário de vencimentos e Alelo | RF-31 a RF-42, RN-14 a RN-16 | Todas as contas da casa e os quatro financiamentos aparecem no calendário com saldo devedor correto |
| 4. Metas e terceiros | Caixinhas e valores a receber | RF-24 a RF-30, RF-43 a RF-49, RN-04 a RN-06, RN-09 a RN-12 | A compra dividida do ar-condicionado mostra cotas, pagamentos e parte líquida corretos |
| 5. Visão geral | Dashboard completo, gráficos, orçamentos e notificações | RF-51, RF-52, RF-58 a RF-64, seção 5.3 | O saldo previsto do mês confere com a conta feita à mão |
| 6. Importação | CSV do Nubank com regras de apelido e detecção de duplicatas | RF-53 a RF-57, RN-19, RN-20 | Uma fatura real importada sem duplicatas e com os apelidos aplicados |
| 7. Mercado | Listas, comparação de preços e leitura da NFC-e | RF-65 a RF-77, RN-22 a RN-26 | Uma lista real mostra ranking e melhor divisão entre mercados a partir de notas escaneadas |
| 8. Grupos e entrada | Grupos, membros, PIN, grupo ativo, administração, grupo em todas as tabelas e consultas, Mercado por grupo, avisos ao vivo por grupo | RF-01 a RF-03, RF-78 a RF-82, RF-85, RN-27 | O casal entra com PIN e vê tudo como antes, e um segundo grupo de teste não enxerga nada do primeiro em nenhuma tela nem rota |
| 9. Contas da pessoa e lançamentos pessoais | Lançamentos pessoais ou do grupo, visibilidade do pessoal só para o dono, relatórios por grupo | RF-83 | Uma compra da casa de praia feita no cartão do casal aparece só no grupo da casa de praia, e a fatura completa só para a dona do cartão |
| 10. Acerto entre membros | Pesos, cotas de membros por despesa e por parcela, saldos e pagamentos entre membros | RF-84, RN-28 a RN-31 | Um exemplo com três membros fecha centavo a centavo e zera depois dos pagamentos |
| 11. Textos e ponta a ponta | Os textos "casal" viram o nome do grupo; e2e com dois grupos e uma pessoa nos dois | Todos os desta lista | Dois grupos leem a mesma nota fiscal sem misturar produtos nem preços, e nenhuma tela fala em casal |
| 12. Android | PWA no Chrome do Android: certificado e acesso pelo IP, instalação na tela inicial, voltar fechando folhas e diálogos, vibração pelo navegador, barra do sistema no tema, textos sem citar só o iPhone e Playwright com viewport de Android | RF-68, RNF-20 a RNF-26, seção 5.5 | Um Android instala o Radar pelo Chrome, entra com PIN, lança uma compra em até quatro toques, lê uma nota pela câmera e o voltar fecha as folhas sem sair da tela, com o iPhone funcionando como antes |

## 10. Fora do escopo da v1

Estes itens ficam registrados para versões futuras e não entram na primeira entrega.

- Acesso fora da rede local, a ser planejado com VPN e revisão de segurança;
- Integração com Open Finance;
- Importação de CSV de outros bancos além do Nubank;
- Importação de histórico anterior ao início do uso;
- Lançamento offline no celular com sincronização posterior;
- Notificações fora do sistema, como e-mail ou push;
- Registro de salário bruto e descontos de CLT;
- Acompanhamento de investimentos além do saldo das caixinhas;
- Sair de um grupo com saldo de acerto em aberto;
- Categorias próprias de cada grupo, além das categorias comuns;
- Rendas pessoais na base de cálculo das caixinhas de um grupo.

### Próximos passos

- [ ] Revisar este documento e ajustar requisitos;
- [ ] Definir critérios de aceitação de cada requisito;
- [x] Escolher a stack técnica;
- [x] Definir o recorte do MVP e a ordem de construção dos módulos.
