# Fut App API

API Go do Fut App, responsavel por autenticacao, partidas, cobrancas, votacao e midias.

## Desenvolvimento

```bash
docker network create fut-app
docker compose up -d postgres
go run ./cmd/api
```

Copie `.env.example` para `.env` ao executar por Docker. A PWA e publicada por um repositorio separado e alcanca esta API pela rede Docker externa `fut-app`.

## Verificacao

```bash
go test ./...
go vet ./...
staticcheck ./...
```
