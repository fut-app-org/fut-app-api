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

## Dados de demonstração

Para revisar a interface com os usuários ativos já existentes, use o script operacional abaixo no servidor. Ele não cria usuários e não altera perfis, senhas ou configurações. As partidas, votações, cobranças e atividades recebem um marcador exclusivo e podem ser removidas integralmente.

```bash
bash scripts/demo-data.sh seed
bash scripts/demo-data.sh reset
```

Por padrão o script usa `/opt/fut-app-api`. Em outro ambiente, informe o diretório do projeto em `FUT_APP_API_DIR`.
