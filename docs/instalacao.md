# Instalação do servidor em casa

Passo a passo para deixar o Radar rodando no Windows e aberto como app nos iPhones (seções 8.3 a 8.5).

## 1. Máquina

| Item | Como fazer | Feito |
| --- | --- | --- |
| WSL2 com Ubuntu 24.04 | `wsl --install -d Ubuntu-24.04`. O repositório fica em `/home/<usuario>/radar-finance`, nunca em `C:` | [x] |
| Docker Desktop | Backend WSL2, integração ligada para o Ubuntu em Settings, Resources, WSL integration | [x] |
| Docker inicia com o Windows | Settings, General, "Start Docker Desktop when you sign in" (RNF-02) | [x] |
| VS Code com Dev Containers | Extensões "WSL" e "Dev Containers" | [ ] |

## 2. Windows como servidor (seção 8.5)

**Login automático.** O Docker Desktop só sobe depois do login. Sem login automático, o Radar fica fora do ar depois de um reinício.

1. Configurações, Contas, Opções de entrada: desligue "Para maior segurança, permitir apenas a entrada do Windows Hello".
2. Tecla Windows + R, digite `netplwiz`, desmarque "Os usuários devem digitar um nome de usuário e uma senha" e confirme com a senha da conta.

**Sem suspensão.** No PowerShell como administrador:

```powershell
powercfg /change standby-timeout-ac 0
powercfg /change hibernate-timeout-ac 0
```

**Windows Update.** Configurações, Windows Update, Opções avançadas, Horário ativo: das 6h às 23h, para que os reinícios aconteçam de madrugada.

**Rede privada.** Configurações, Rede e Internet, propriedades da rede de casa: marque **Rede privada**.

**Firewall.** No PowerShell como administrador, libere as portas só para redes privadas:

```powershell
New-NetFirewallRule -DisplayName "Radar HTTPS" -Direction Inbound -Protocol TCP -LocalPort 443 -Profile Private -Action Allow
New-NetFirewallRule -DisplayName "Radar HTTP redirect" -Direction Inbound -Protocol TCP -LocalPort 80 -Profile Private -Action Allow
```

Nenhuma porta é aberta no roteador (RNF-08).

**Endereço fixo (RNF-22).** No roteador, reserve o IP atual da máquina para o endereço MAC da placa de rede (DHCP reservation). Prefira o cabo Ethernet ao Wi-Fi. O nome `<NOMEDOPC>.local` também funciona no iPhone, porque o Windows responde por mDNS.

**Pasta dos backups.** Crie `C:\RadarBackups`. Se um dia houver um segundo disco, mude para ele no `deploy/.env`.

## 3. Certificado HTTPS (RNF-21)

Dentro do Dev Container, gere a autoridade local e o certificado com o nome e o IP da máquina:

```bash
make certs HOSTS="nomedopc.local 192.168.0.10"
```

Isso cria `deploy/certs/`, que não vai para o repositório. Guarde a pasta `deploy/certs/ca`: ela tem a chave da autoridade local.

**Instalar a autoridade em cada iPhone, uma vez só** (depois do passo 4, com o Radar no ar):

1. No **Safari** do iPhone, abra `http://<IP do servidor>/ca.crt` (http, sem o s) e toque em **Permitir**. Abrir o arquivo pelo WhatsApp não funciona: o iOS só mostra o texto.
2. Ajustes: logo no topo aparece **Perfil Transferido**. Se não aparecer, vá em Geral, VPN e Gerenciamento de Dispositivo. Toque em Instalar. O perfil baixado expira em alguns minutos; se sumir, baixe de novo.
3. Ajustes, Geral, Sobre, Ajustes de Confiança de Certificados: ative a confiança total para "mkcert".

A pasta `deploy/certs/ca` guarda a chave da autoridade e nunca sai do PC; o Caddy recebe só os certificados públicos.

## 4. Subir o Radar

No terminal do Ubuntu, na pasta do repositório:

```bash
cp deploy/.env.example deploy/.env   # ajuste RADAR_HOSTS e RADAR_BACKUP_HOST_DIR
make deploy
make deploy-user-add LOGIN=<login> NAME="<nome>"   # a primeira pessoa
make deploy-group-create NAME="<grupo>" ADMIN=<login>   # o primeiro grupo, com ela como administradora
make deploy-user-add LOGIN=<login> NAME="<nome>" GROUP="<grupo>"   # as outras pessoas do grupo
make deploy-user-pin LOGIN=<login>   # o PIN de cada pessoa, de 4 a 6 números, digitado no terminal
```

Depois disso, os administradores do grupo cadastram pessoas, criam outros grupos e redefinem PINs pelo próprio Radar, em Ajustes.

**Atualizando um Radar de antes da Fase 8.** A atualização cria o grupo "Casa" com as duas pessoas e todos os dados, mas ninguém tem PIN ainda, então ninguém entra até o PIN ser definido pelo terminal:

```sh
make deploy-user-pin LOGIN=<login>   # uma vez para cada pessoa
```

Os usuários não têm senha (RF-01) e ficam só no banco, nunca no repositório. Atualizações futuras usam o mesmo `make deploy` (RNF-19). As migrações rodam sozinhas.

## 5. Instalar como app no iPhone (RNF-23)

1. No Safari, abra `https://nomedopc.local` ou `https://192.168.0.10`.
2. Toque em Compartilhar, Adicionar à Tela de Início.
3. Abra pelo ícone: o Radar abre em tela cheia, sem a barra do Safari.

## 6. Conferir

- Ajustes, Saúde do sistema: mostra o último backup e há quanto tempo o servidor está ligado.
- `C:\RadarBackups` recebe uma cópia por dia.
- Depois de reiniciar o Windows, o Radar volta sozinho.
