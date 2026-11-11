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

**Instalar a autoridade em cada iPhone, uma vez só:**

1. Envie `deploy/certs/radar-root-ca.pem` para o iPhone por AirDrop ou e-mail e abra o arquivo.
2. Ajustes, Perfil Transferido, Instalar.
3. Ajustes, Geral, Sobre, Ajustes de Confiança de Certificados: ative a confiança total para "mkcert".

## 4. Subir o Radar

No terminal do Ubuntu, na pasta do repositório:

```bash
cp deploy/.env.example deploy/.env   # ajuste RADAR_HOSTS e RADAR_BACKUP_HOST_DIR
make deploy
make deploy-user-add LOGIN=italo NAME="Ítalo"
make deploy-user-add LOGIN=<login> NAME="<nome>"
```

O comando pede a senha duas vezes, sem mostrar na tela. Atualizações futuras usam o mesmo `make deploy` (RNF-19). As migrações rodam sozinhas.

## 5. Instalar como app no iPhone (RNF-23)

1. No Safari, abra `https://nomedopc.local` ou `https://192.168.0.10`.
2. Toque em Compartilhar, Adicionar à Tela de Início.
3. Abra pelo ícone: o Radar abre em tela cheia, sem a barra do Safari.

## 6. Conferir

- Ajustes, Saúde do sistema: mostra o último backup e há quanto tempo o servidor está ligado.
- `C:\RadarBackups` recebe uma cópia por dia.
- Depois de reiniciar o Windows, o Radar volta sozinho.
