package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

const chargeColumns = `
	c.id, c.batch_id, c.user_id, u.name, u.role, u.avatar_color,
	to_char(c.reference_month, 'YYYY-MM'), c.amount_cents, c.status, c.due_date::text,
	c.paid_at, c.paid_method, c.registered_by, coalesce(r.name, ''), c.pix_payload,
	c.pix_ticket_url, c.pix_qr_code_base64, c.created_at`

const chargeJoins = `
	from charges c
	join users u on u.id = c.user_id
	left join users r on r.id = c.registered_by`

func scanCharge(row pgx.Row) (Charge, error) {
	var c Charge
	err := row.Scan(&c.ID, &c.BatchID, &c.UserID, &c.UserName, &c.UserRole, &c.AvatarColor,
		&c.ReferenceMonth, &c.AmountCents, &c.Status, &c.DueDate,
		&c.PaidAt, &c.PaidMethod, &c.RegisteredBy, &c.RegisteredName, &c.PixPayload,
		&c.PixTicketURL, &c.PixQRCodeBase64, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

// SaveMercadoPagoOrder guarda o Pix retornado pelo gateway. A mesma cobrança
// pode ser solicitada mais de uma vez com a mesma idempotency key sem criar
// uma segunda order no Mercado Pago.
func (s *Store) SaveMercadoPagoOrder(ctx context.Context, chargeID, orderID, paymentID, status, statusDetail, pixPayload, ticketURL, qrCodeBase64 string) error {
	tag, err := s.pool.Exec(ctx, `
		update charges
		set payment_provider = 'mercado_pago', provider_order_id = $2, provider_payment_id = $3,
			provider_status = $4, provider_status_detail = $5, pix_payload = $6,
			pix_ticket_url = $7, pix_qr_code_base64 = $8
		where id = $1 and (provider_order_id = '' or provider_order_id = $2)`,
		chargeID, orderID, paymentID, status, statusDetail, pixPayload, ticketURL, qrCodeBase64)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateMercadoPagoStatus registra o último estado informado pelo gateway.
func (s *Store) UpdateMercadoPagoStatus(ctx context.Context, chargeID, orderID, paymentID, status, statusDetail string) error {
	tag, err := s.pool.Exec(ctx, `
		update charges
		set provider_payment_id = $3, provider_status = $4, provider_status_detail = $5
		where id = $1 and payment_provider = 'mercado_pago' and provider_order_id = $2`,
		chargeID, orderID, paymentID, status, statusDetail)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// MarkMercadoPagoPaid confirma um Pix aprovado pelo gateway. Retornos de
// webhook podem ser repetidos, portanto a função informa se esta chamada fez
// a baixa pela primeira vez.
func (s *Store) MarkMercadoPagoPaid(ctx context.Context, chargeID, orderID, paymentID string) (Charge, bool, bool, error) {
	tag, err := s.pool.Exec(ctx, `
		update charges
		set status = 'paid', paid_at = now(), paid_method = 'pix', provider_payment_id = $3,
			provider_status = 'processed', provider_status_detail = 'accredited'
		where id = $1 and payment_provider = 'mercado_pago' and provider_order_id = $2
		  and status in ('pending', 'overdue')`, chargeID, orderID, paymentID)
	if err != nil {
		return Charge{}, false, false, err
	}

	charge, err := s.ChargeByID(ctx, chargeID)
	if err != nil {
		return Charge{}, false, false, err
	}
	if tag.RowsAffected() == 0 {
		return charge, false, false, nil
	}

	reactivated, err := s.reactivateIfClear(ctx, charge.UserID, "")
	return charge, true, reactivated, err
}

func (s *Store) ChargeByID(ctx context.Context, id string) (Charge, error) {
	return scanCharge(s.pool.QueryRow(ctx, `select `+chargeColumns+chargeJoins+` where c.id = $1`, id))
}

func (s *Store) ChargesByUser(ctx context.Context, userID string) ([]Charge, error) {
	rows, err := s.pool.Query(ctx,
		`select `+chargeColumns+chargeJoins+` where c.user_id = $1 order by c.reference_month desc`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectCharges(rows)
}

// ChargesByMonth lista as cobranças do mês (YYYY-MM); vazio lista o lote mais recente.
func (s *Store) ChargesByMonth(ctx context.Context, month string) ([]Charge, error) {
	query := `select ` + chargeColumns + chargeJoins
	args := []any{}
	if month != "" {
		query += ` where to_char(c.reference_month, 'YYYY-MM') = $1`
		args = append(args, month)
	} else {
		query += ` where c.reference_month = (select max(reference_month) from charge_batches)`
	}
	query += ` order by u.name`

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectCharges(rows)
}

func collectCharges(rows pgx.Rows) ([]Charge, error) {
	var charges []Charge
	for rows.Next() {
		c, err := scanCharge(rows)
		if err != nil {
			return nil, err
		}
		charges = append(charges, c)
	}
	return charges, rows.Err()
}

func (s *Store) BatchByMonth(ctx context.Context, month string) (ChargeBatch, error) {
	query := `
		select b.id, to_char(b.reference_month, 'YYYY-MM'), b.total_amount_cents, b.user_count,
		       b.individual_amount_cents, b.due_date::text, b.generated_by, u.name, b.created_at
		from charge_batches b join users u on u.id = b.generated_by`
	var row pgx.Row
	if month != "" {
		row = s.pool.QueryRow(ctx, query+` where to_char(b.reference_month, 'YYYY-MM') = $1`, month)
	} else {
		row = s.pool.QueryRow(ctx, query+` order by b.reference_month desc limit 1`)
	}
	var b ChargeBatch
	err := row.Scan(&b.ID, &b.ReferenceMonth, &b.TotalAmountCents, &b.UserCount,
		&b.IndividualAmountCents, &b.DueDate, &b.GeneratedBy, &b.GeneratedByName, &b.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return b, ErrNotFound
	}
	return b, err
}

var ErrBatchExists = errors.New("já existe cobrança gerada para este mês")

// GenerateBatch cria o lote do mês e uma cobrança por usuário ativo, registrando a
// fotografia do rateio (valor total, quantidade e valor individual fixos).
func (s *Store) GenerateBatch(ctx context.Context, month string, totalCents int64, dueDate time.Time, generatedBy string) (ChargeBatch, error) {
	users, err := s.ActiveUsers(ctx)
	if err != nil {
		return ChargeBatch{}, err
	}
	if len(users) == 0 {
		return ChargeBatch{}, errors.New("nenhum usuário ativo para ratear")
	}
	individual := totalCents / int64(len(users))

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ChargeBatch{}, err
	}
	defer tx.Rollback(ctx)

	var exists bool
	if err := tx.QueryRow(ctx,
		`select exists (select 1 from charge_batches where reference_month = ($1 || '-01')::date)`, month).Scan(&exists); err != nil {
		return ChargeBatch{}, err
	}
	if exists {
		return ChargeBatch{}, ErrBatchExists
	}

	var batchID string
	err = tx.QueryRow(ctx, `
		insert into charge_batches (reference_month, total_amount_cents, user_count, individual_amount_cents, due_date, generated_by)
		values (($1 || '-01')::date, $2, $3, $4, $5, $6) returning id`,
		month, totalCents, len(users), individual, dueDate, generatedBy).Scan(&batchID)
	if err != nil {
		return ChargeBatch{}, err
	}

	for _, u := range users {
		_, err := tx.Exec(ctx, `
			insert into charges (batch_id, user_id, reference_month, amount_cents, due_date)
			values ($1, $2, ($3 || '-01')::date, $4, $5)`,
			batchID, u.ID, month, individual, dueDate)
		if err != nil {
			return ChargeBatch{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return ChargeBatch{}, err
	}
	return s.BatchByMonth(ctx, month)
}

// MarkChargePaid registra o pagamento e, se o usuário estava inativo por
// inadimplência e não deve mais nada vencido, o reativa automaticamente.
func (s *Store) MarkChargePaid(ctx context.Context, chargeID, method string, registeredBy string) (Charge, bool, error) {
	status := "manual_paid"
	if method == "pix" {
		status = "paid"
	}
	tag, err := s.pool.Exec(ctx, `
		update charges set status = $2, paid_at = now(), paid_method = $3, registered_by = $4
		where id = $1 and status in ('pending', 'overdue')`,
		chargeID, status, method, registeredBy)
	if err != nil {
		return Charge{}, false, err
	}
	if tag.RowsAffected() == 0 {
		return Charge{}, false, ErrNotFound
	}

	charge, err := s.ChargeByID(ctx, chargeID)
	if err != nil {
		return Charge{}, false, err
	}

	reactivated, err := s.reactivateIfClear(ctx, charge.UserID, registeredBy)
	return charge, reactivated, err
}

func (s *Store) reactivateIfClear(ctx context.Context, userID, changedBy string) (bool, error) {
	var eligible bool
	err := s.pool.QueryRow(ctx, `
		select u.status = 'inactive'
		   and not exists (select 1 from charges c where c.user_id = u.id and c.status = 'overdue')
		from users u where u.id = $1`, userID).Scan(&eligible)
	if err != nil || !eligible {
		return false, err
	}
	by := &changedBy
	if changedBy == "" {
		by = nil
	}
	return true, s.ChangeUserStatus(ctx, userID, "active", "pagamento confirmado", by)
}

func (s *Store) SetChargeStatus(ctx context.Context, chargeID, newStatus string) error {
	tag, err := s.pool.Exec(ctx, `
		update charges set status = $2 where id = $1 and status in ('pending', 'overdue')`, chargeID, newStatus)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// MarkOverdueCharges marca como vencidas as cobranças pendentes após o vencimento
// e retorna os usuários afetados.
func (s *Store) MarkOverdueCharges(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		update charges set status = 'overdue'
		where status = 'pending' and due_date < current_date
		returning user_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectIDs(rows)
}

// UsersToInactivate lista usuários ativos com cobrança vencida há mais de graceDays.
func (s *Store) UsersToInactivate(ctx context.Context, graceDays int) ([]User, error) {
	rows, err := s.pool.Query(ctx, `
		select `+userColumns+` from users u
		where u.status = 'active' and exists (
			select 1 from charges c
			where c.user_id = u.id and c.status = 'overdue'
			  and c.due_date < current_date - make_interval(days => $1)::interval
		)`, graceDays)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}
