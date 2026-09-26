# AGENTS.md — fut-app-api

API do sistema **Fut da Rapaziada** — gestão de partidas, mensalidades e votação.

## Stack

Go 1.26+ · chi router · PostgreSQL · sqlc/sqlx · JWT httpOnly · robfig/cron

## Estrutura

```
cmd/api/            binário do servidor
internal/
  api/              rotas, middlewares e handlers HTTP
  auth/             hash de senha, JWT de sessão, rate limit de login
  busdays/          prazos em dias úteis (5º dia útil, lembrete D-1)
  config/           configuração por variáveis de ambiente
  draw/             sorteio dos times
  jobs/             cron: fechar confirmações/votação, vencer cobranças, lembretes
  notify/           envio de WhatsApp (stub até definir provedor)
  store/            PostgreSQL: migrations embutidas, queries e modelos
```

## Comandos

```bash
go test ./...            # testes (dias úteis, balanceamento do sorteio)
go vet ./...             # lint estático
go build ./cmd/api       # compila
```

## Executar localmente

```bash
DATABASE_URL="postgres://futapp:futapp@localhost:5432/futapp?sslmode=disable" \
JWT_SECRET="dev-secret" \
SEED_ADMIN_EMAIL="admin@local" SEED_ADMIN_PASSWORD="admin12345" \
go run ./cmd/api
```

## Repo frontend

O frontend (Vue 3 + Vite) vive em `fut-app-org/fut-app-pwa`. Este repo e o frontend são versionados e deployados independentemente.

## Documentação completa

Ver `../README.md`, `../STACK.md` e `../requisitos.md` no workspace raiz (`fut-app/`).
