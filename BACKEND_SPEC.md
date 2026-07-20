# Fut da Rapaziada — Especificação do Backend (Go)

## 1. Visão geral

API REST em Go servindo o SPA Vue. Autenticação por convite (sem cadastro aberto), gestão de partidas/confirmações/sorteio de times, cobrança recorrente via PIX, votação pós-partida e painel administrativo.

**Stack:** Go (`chi`), PostgreSQL, `sqlc`, JWT (cookie httpOnly), `robfig/cron`, gateway PIX externo, WhatsApp API.

## 2. Modelo de dados (PostgreSQL)

```sql
users (
  id uuid pk, name text, email text unique, phone text,
  password_hash text, avatar_color text,
  role text check (role in ('admin','player')) default 'player',
  status text check (status in ('active','inactive')) default 'active',
  created_at timestamptz
)

invites (
  id uuid pk, token text unique, invited_name text,
  created_by uuid references users(id),
  role text default 'player',
  expires_at timestamptz, used_at timestamptz, used_by uuid references users(id),
  revoked_at timestamptz, created_at timestamptz
)

matches (
  id uuid pk, match_date date, start_time time, end_time time,
  venue text, address text,
  confirmation_deadline timestamptz,
  status text check (status in ('open','closed','teams_drawn','finished','cancelled')),
  cancel_reason text,
  created_at timestamptz
)

match_confirmations (
  match_id uuid references matches(id), user_id uuid references users(id),
  response text check (response in ('going','not_going','no_response')) default 'no_response',
  responded_at timestamptz,
  primary key (match_id, user_id)
)

match_teams (
  id uuid pk, match_id uuid references matches(id),
  team_name text, team_color text
)
match_team_members (
  team_id uuid references match_teams(id), user_id uuid references users(id),
  primary key (team_id, user_id)
)

charges (
  id uuid pk, reference_month date, user_id uuid references users(id),
  amount_cents int, status text check (status in ('pending','paid','manual_paid')) default 'pending',
  pix_payload text, pix_txid text,
  due_date date, paid_at timestamptz, paid_method text,
  registered_by uuid references users(id) null,
  created_at timestamptz
)

match_media (
  id uuid pk, match_id uuid references matches(id), uploaded_by uuid,
  type text check (type in ('photo','video')), url text, created_at timestamptz
)

votes (
  id uuid pk, match_id uuid references matches(id), voter_id uuid references users(id),
  category text check (category in ('top_scorer','worst_player')),
  candidate_id uuid references users(id),
  unique (match_id, voter_id, category)
)
```

## 3. Endpoints REST

**Auth / convite**
- `GET /api/invites/:token` — valida convite (nome, expiração)
- `POST /api/invites/:token/signup` — cria conta a partir do convite
- `POST /api/login` — email+senha → seta cookie JWT
- `POST /api/logout`

**Início / partida**
- `GET /api/matches/next` — dados da próxima partida + contagem de confirmações
- `POST /api/matches/:id/confirm` — body `{response: going|not_going}`
- `GET /api/matches/:id/confirmations` — listas indo/não vão/sem resposta
- `POST /api/matches/:id/close-confirmations` (admin)
- `POST /api/matches/:id/draw-teams` (admin) — sorteia e grava `match_teams`
- `GET /api/matches/:id/teams`

**Pagamentos**
- `GET /api/charges/me` — cobrança do mês do usuário logado + histórico
- `POST /api/charges/:id/pix` — gera/retorna payload PIX (copia-e-cola) + QR
- `POST /api/webhooks/pix` — callback do gateway, marca `charges.status='paid'`

**Histórico / votação**
- `GET /api/matches?month=` — lista de partidas
- `GET /api/matches/:id` — detalhe + status de votação
- `POST /api/matches/:id/votes` — body `{category, candidate_id}`
- `GET /api/matches/:id/media`, `POST /api/matches/:id/media`

**Perfil**
- `GET /api/me`, `PATCH /api/me`

**Admin**
- `GET /api/admin/dashboard`
- `GET /api/admin/users`, `PATCH /api/admin/users/:id` (status/role)
- `POST /api/admin/invites`, `GET /api/admin/invites`, `POST /api/admin/invites/:id/revoke`
- `GET /api/admin/charges?month=`, `POST /api/admin/charges/generate` (body `{total_amount, month}` — divide por usuários ativos), `POST /api/admin/charges/:id/mark-paid`

## 4. Regras de negócio chave

- Rateio: `amount_cents = total_amount_cents / count(active_users)` no momento da geração — **fotografia fixa**, não recalcula se o número de ativos mudar depois.
- Vencimento: 5º dia útil após geração da cobrança.
- Inadimplência: cron diário marca `status='inactive'` em usuários com cobrança vencida há mais de N dias (configurável); reativa ao pagar.
- Confirmação: fecha automaticamente no `confirmation_deadline`; sorteio de times só disponível após fechamento.
- Votação: abre ao final da partida, fecha em X dias (configurável), sem voto em si mesmo na categoria "perna de pau".

## 5. Jobs agendados (`robfig/cron`)

| Job | Frequência | Ação |
|---|---|---|
| Gerar cobrança do mês | mensal (config admin dispara manual, mas cron pode lembrar) | cria `charges` para ativos |
| Lembrete WhatsApp | diário | notifica cobranças a X dias do vencimento |
| Inativar inadimplentes | diário | atualiza `users.status` |
| Fechar confirmações | a cada minuto (checa deadlines) | `matches.status='closed'` |
| Fechar votação | diário | trava `votes` após prazo |

## 6. Integrações externas

- **Gateway PIX** (Mercado Pago / Efí / Asaas): usado para cobrança dinâmica com webhook de confirmação automática — ver seção 7 para o modo estático próprio.
- **WhatsApp**: Twilio API ou Meta Cloud API, endpoint interno `internal/notify/whatsapp.go` com fila simples (tabela `notifications` + worker).

---

# 7. Ferramenta de geração de PIX (BR Code / EMV)

Objetivo: gerar o payload "copia e cola" (BR Code, padrão EMV do Banco Central) **sem depender de gateway** para o caso de PIX estático (chave fixa, valor variável por cobrança) — útil como fallback ou modo principal caso não queira taxas de gateway. Para confirmação automática de pagamento ainda é necessário um gateway ou webhook do seu PSP (o BR Code puro não avisa quando foi pago).

## 7.1 Formato do payload (TLV — Tag/Length/Value)

Estrutura mínima exigida pelo BACEN:

| ID | Campo | Obrigatório |
|---|---|---|
| 00 | Payload Format Indicator (`01`) | sim |
| 26 | Merchant Account Info (sub: 00 GUI `br.gov.bcb.pix`, 01 chave PIX, 02 descrição opcional) | sim |
| 52 | Merchant Category Code (`0000`) | sim |
| 53 | Moeda (`986` = BRL) | sim |
| 54 | Valor da transação (ex: `80.00`) | opcional (dinâmico se ausente) |
| 58 | País (`BR`) | sim |
| 59 | Nome do recebedor (máx 25 char) | sim |
| 60 | Cidade do recebedor (máx 15 char) | sim |
| 62 | Additional Data (sub 05: txid) | sim |
| 63 | CRC16 (checksum, sempre últimos 4 chars) | sim |

## 7.2 Implementação em Go

```go
package pix

import (
	"fmt"
	"strings"
)

type ChargeInput struct {
	PixKey      string // chave pix do grupo (CPF/CNPJ/email/telefone/aleatória)
	MerchantName string // nome do recebedor, máx 25 char, sem acento
	MerchantCity string // cidade, máx 15 char
	TxID        string // identificador único da cobrança (ex: charge id curto)
	AmountCents int64  // 0 = valor livre (usuário digita no banco)
}

func tlv(id string, value string) string {
	return fmt.Sprintf("%s%02d%s", id, len(value), value)
}

func GeneratePayload(in ChargeInput) string {
	merchantAccount := tlv("00", "br.gov.bcb.pix") + tlv("01", in.PixKey)
	payload := tlv("00", "01") +
		tlv("26", merchantAccount) +
		tlv("52", "0000") +
		tlv("53", "986")

	if in.AmountCents > 0 {
		amount := fmt.Sprintf("%.2f", float64(in.AmountCents)/100)
		payload += tlv("54", amount)
	}

	payload += tlv("58", "BR") +
		tlv("59", truncate(in.MerchantName, 25)) +
		tlv("60", truncate(in.MerchantCity, 15)) +
		tlv("62", tlv("05", truncate(in.TxID, 25)))

	payload += "6304" // placeholder do CRC, recalculado abaixo
	crc := crc16CCITT(payload)
	return payload + crc
}

func truncate(s string, n int) string {
	s = strings.ToUpper(s)
	if len(s) > n {
		return s[:n]
	}
	return s
}

func crc16CCITT(payload string) string {
	var crc uint16 = 0xFFFF
	for _, b := range []byte(payload) {
		crc ^= uint16(b) << 8
		for i := 0; i < 8; i++ {
			if crc&0x8000 != 0 {
				crc = (crc << 1) ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return fmt.Sprintf("%04X", crc)
}
```

Uso:

```go
payload := pix.GeneratePayload(pix.ChargeInput{
	PixKey:       "grupo@futdarapaziada.com",
	MerchantName: "Fut da Rapaziada",
	MerchantCity: "Sao Paulo",
	TxID:         "CHG20260716RS",
	AmountCents:  8000, // R$ 80,00
})
// payload -> string "copia e cola" pronta pra virar QR code
```

## 7.3 Gerando o QR code a partir do payload

Use `github.com/skip2/go-qrcode` no backend (retorna PNG) ou gere no front com `qrcode` (npm) a partir do `payload` recebido da API — mais leve, evita processar imagem no Go:

```go
import "github.com/skip2/go-qrcode"

png, err := qrcode.Encode(payload, qrcode.Medium, 256)
```

## 7.4 Endpoint

`GET /api/charges/:id/pix` retorna:
```json
{
  "payload": "00020126580014br.gov.bcb.pix...6304A1B2",
  "amount_cents": 8000,
  "txid": "CHG20260716RS"
}
```
O front renderiza o QR (via lib JS) e mostra o botão "copiar" com o `payload`.

## 7.5 Limitação e recomendação

BR Code estático **não confirma pagamento automaticamente** — alguém (admin) precisa registrar manualmente ("Registrar pagamento", já previsto na tela de Mensalidades) ou você integra um gateway (Mercado Pago/Efí/Asaas) que devolve `txid` + webhook de confirmação em tempo real. Recomendo: lançar com BR Code próprio (custo zero) + confirmação manual pelo admin no MVP, migrar para gateway com webhook quando o volume justificar automação.
