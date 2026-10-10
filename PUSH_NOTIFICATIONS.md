# Avisos de partidas por Web Push

## Comportamento

Jogadores habilitam notificações em **Perfil → Notificações de partidas**, por aparelho. O app pede permissão apenas ao tocar em Ativar. Usuários ativos com inscrição recebem um aviso quando a partida é criada; a fila é gravada na mesma transação da partida. Um worker executa a cada minuto, portanto o envio pode levar aproximadamente um minuto.

O lembrete padrão é enviado na janela das duas horas anteriores ao prazo de confirmação, somente a jogadores ativos que ainda não responderam. Não há lembrete nos primeiros cinco minutos depois da criação. `MATCH_REMINDER_HOURS=24` altera a janela para um dia; `0` desativa lembretes. O prazo atual é consultado ao enviar, respeitando edições. Partidas canceladas, confirmações fechadas ou prazos vencidos não geram novos envios. Uma notificação já entregue ao serviço do navegador não pode ser retirada caso a partida seja cancelada depois.

Há um envio por aparelho/partida/tipo, retries com backoff (até cinco tentativas) e lease de dois minutos para recuperar workers interrompidos. HTTP 404/410 remove inscrições expiradas. HTTP 429/5xx e falhas de rede permitem retry. Como em qualquer fila com entrega pelo menos uma vez, uma interrupção após o serviço aceitar um push pode ocasionar reenvio; a tag agrupa duplicatas no aparelho.

Tocar no aviso abre a partida específica para responder presença. O redirect é preservado se for necessário fazer login. Desativar ou sair da conta remove a inscrição do aparelho. Um aparelho pode se inscrever em apenas uma conta por vez.

## Compatibilidade

Use HTTPS em produção. No iPhone/iPad (iOS/iPadOS 16.4+), adicione o app à tela inicial pelo Safari e abra o ícone para habilitar notificações. Em navegadores compatíveis no Android, permita notificações. Entrega depende de permissões, conectividade, navegador e configurações do sistema; não é uma garantia de leitura.

Fontes: https://webkit.org/blog/13878/web-push-for-web-apps-on-ios-and-ipados/ e https://developer.mozilla.org/en-US/docs/Web/API/Push_API.

## Configuração da API

Gere uma única dupla VAPID, guardando a chave privada fora do Git e dos logs:

```bash
umask 077
go run ./cmd/vapid > /tmp/fut-app-vapid.env
```

Configure no ambiente do container da API:

```dotenv
VAPID_PUBLIC_KEY=<chave-publica>
VAPID_PRIVATE_KEY=<chave-privada>
VAPID_SUBJECT=mailto:<email-do-responsavel>
MATCH_REMINDER_HOURS=2
```

Mantenha as chaves entre deploys; trocar a pública exige nova inscrição dos aparelhos. Sem as três variáveis VAPID, a API continua funcionando e o frontend informa que as notificações ainda não estão disponíveis. A chave pública é obtida pela PWA via API: nenhum segredo precisa ser colocado no build do frontend ou no GitHub Actions.

`DISABLE_JOBS=true` desativa também os envios push na instância local, deixando o cron da VPS operar o banco compartilhado. Em produção, deixe a variável ausente ou como `false`.

## CI/CD e ordem de publicação

1. Revisar os PRs da API e da PWA. A migration `008_match_push.sql` é aditiva; o código anterior continua compatível.
2. Preparar as variáveis VAPID no `.env` da VPS **e no mapeamento `environment` do Compose efetivamente usado no deploy**. O Compose deste repositório inclui esse mapeamento, mas o Compose da VPS é mantido separadamente. Somente acrescentar chaves ao `.env` não as injeta no container.
3. Publicar a API pelo merge do PR. O startup aplica a migration e registra o worker quando as chaves estão configuradas.
4. Publicar a PWA pelo merge do PR. O novo service worker importa `/push-sw.js`; o Caddy evita cache persistente desse script.
5. No próprio aparelho, ativar notificações no Perfil e verificar o aviso na próxima criação real de partida. Nunca testar disparos com as inscrições de outros jogadores.

Rollback: o código anterior da API/PWA funciona com as tabelas adicionais. Não apagar o banco nem desfazer a migration para reverter a aplicação.

## Testes

```bash
go test ./...
go vet ./...
TEST_DATABASE_URL='postgres://.../futapp_test?sslmode=disable' go test ./internal/store -v
```

Os testes de banco criam e removem schemas isolados, sem consultar usuários reais. Cobrem público ativo, deduplicação, ownership, cancelamento, respostas, mudança de prazo, retries e inscrições expiradas. O CI inclui PostgreSQL 17 e executa esses testes. O transporte push é simulado nos testes: nenhum aviso é enviado a aparelhos.
