package store

import "context"

type Dashboard struct {
	ActiveUsers    int   `json:"active_users"`
	InactiveUsers  int   `json:"inactive_users"`
	Delinquents    int   `json:"delinquents"`
	PendingInvites int   `json:"pending_invites"`
	MonthPaidCents int64 `json:"month_paid_cents"`
	MonthDueCents  int64 `json:"month_due_cents"`
	PaidCount      int   `json:"paid_count"`
	PendingCount   int   `json:"pending_count"`
	ChargeCount    int   `json:"charge_count"`
}

// DashboardStats agrega os indicadores do painel admin. Os valores financeiros
// referem-se ao lote de cobrança mais recente.
func (s *Store) DashboardStats(ctx context.Context) (Dashboard, error) {
	var d Dashboard
	err := s.pool.QueryRow(ctx, `
		select
			(select count(*) from users where status = 'active'),
			(select count(*) from users where status = 'inactive'),
			(select count(distinct user_id) from charges where status = 'overdue'),
			(select count(*) from invites where used_at is null and revoked_at is null and expires_at > now())`).
		Scan(&d.ActiveUsers, &d.InactiveUsers, &d.Delinquents, &d.PendingInvites)
	if err != nil {
		return d, err
	}
	err = s.pool.QueryRow(ctx, `
		select
			coalesce(sum(amount_cents) filter (where status in ('paid', 'manual_paid')), 0),
			coalesce(sum(amount_cents) filter (where status not in ('cancelled', 'exempt')), 0),
			count(*) filter (where status in ('paid', 'manual_paid')),
			count(*) filter (where status in ('pending', 'overdue')),
			count(*)
		from charges
		where reference_month = (select max(reference_month) from charge_batches)`).
		Scan(&d.MonthPaidCents, &d.MonthDueCents, &d.PaidCount, &d.PendingCount, &d.ChargeCount)
	return d, err
}

// DelinquentUsers lista usuários com cobrança vencida, para o card "Atenção".
func (s *Store) DelinquentUsers(ctx context.Context) ([]Charge, error) {
	rows, err := s.pool.Query(ctx,
		`select `+chargeColumns+chargeJoins+` where c.status = 'overdue' order by c.due_date`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectCharges(rows)
}
