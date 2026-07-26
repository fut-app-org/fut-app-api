# OperaÃ§Ãµes em produÃ§Ã£o

## Backup do PostgreSQL

O backup Ã© um arquivo no formato customizado do PostgreSQL. Ele Ã© validado com
`pg_restore --list` antes de ser publicado no diretÃ³rio de destino.

No servidor, execute manualmente:

```bash
sudo bash /opt/fut-app-api/scripts/backup-postgres.sh
```

Por padrÃ£o, os arquivos ficam em `/var/backups/fut-app` e os backups com mais
de 14 dias sÃ£o removidos. Ajuste sem editar o script:

```bash
sudo FUT_APP_BACKUP_DIR=/caminho/seguro FUT_APP_BACKUP_RETENTION_DAYS=30 \
  bash /opt/fut-app-api/scripts/backup-postgres.sh
```

Configure um agendamento diÃ¡rio como root. Exemplo, todos os dias Ã s 03:15:

```cron
15 3 * * * bash /opt/fut-app-api/scripts/backup-postgres.sh >> /var/log/fut-app-backup.log 2>&1
```

O diretÃ³rio de backup precisa ser copiado para fora da VPS. Um backup guardado
somente no mesmo servidor nÃ£o protege contra perda da VPS.

## RestauraÃ§Ã£o

A restauraÃ§Ã£o para a API, cria automaticamente um backup de seguranÃ§a e sÃ³
aceita a palavra de confirmaÃ§Ã£o `RESTORE`.

```bash
sudo bash /opt/fut-app-api/scripts/restore-postgres.sh \
  /var/backups/fut-app/fut-app-postgres-AAAAMMDDTHHMMSSZ.dump RESTORE
```

Execute esse procedimento primeiro em ambiente de teste. Ele substitui os
dados atuais do PostgreSQL.

## Monitoramento

`/api/healthz` agora sÃ³ responde `200` quando a API e o PostgreSQL estÃ£o
disponÃ­veis. Para verificaÃ§Ã£o externa ou cron:

```bash
bash /opt/fut-app-api/scripts/healthcheck.sh
```

O script retorna cÃ³digo diferente de zero em falha, o que permite integrÃ¡-lo
a ferramentas como Uptime Kuma, Better Stack ou um cron com alerta.
