# Backup e restauração

Como o Radar guarda as cópias do banco e como voltar uma delas (RNF-05 a RNF-07).

## Como funciona

- O banco fica no volume Docker `radar_radar-data`, no arquivo `/data/radar.db`.
- Todo dia, na hora definida em `RADAR_BACKUP_HOUR` (padrão 3h, horário de Brasília), o serviço grava uma cópia com `VACUUM INTO`. A cópia é consistente mesmo com o sistema em uso.
- Antes de entrar no lugar, cada cópia passa por `PRAGMA quick_check` e é gravada no disco.
- Ao ligar, se a cópia do último horário agendado não existir, o serviço faz uma na hora (RNF-06).
- O botão **Fazer agora**, em Ajustes, gera uma cópia manual (RNF-07). Pelo terminal: `make deploy-backup`.
- Ficam as 30 cópias mais recentes (`RADAR_BACKUP_KEEP`). As mais antigas são apagadas.
- As cópias vão para `RADAR_BACKUP_HOST_DIR`, uma pasta do Windows fora do volume. O padrão é `C:\RadarBackups`.

Nome dos arquivos: `radar-AAAAMMDD-HHMMSS-tipo.db`, com tipo `daily`, `startup` ou `manual`.

A página Ajustes mostra a data do último backup, o próximo agendamento e o último erro, se houver.

## Restaurar uma cópia

1. Abra o terminal do Ubuntu (WSL2) e entre na pasta `deploy` do repositório.
2. Escolha o arquivo em `C:\RadarBackups`.
3. Rode:

   ```bash
   ./restore.sh radar-20261110-030000-daily.db
   ```

O script para o Radar, guarda o banco atual como `radar.db.antes-da-restauracao` dentro do volume, copia o backup, ajusta o dono do arquivo e sobe o Radar de novo. As migrações rodam sozinhas na inicialização, então uma cópia antiga é atualizada para o esquema atual.

## Conferir uma cópia sem restaurar

Dentro do Dev Container:

```bash
sqlite3 /caminho/para/radar-20261110-030000-daily.db "PRAGMA integrity_check; SELECT count(*) FROM users;"
```
