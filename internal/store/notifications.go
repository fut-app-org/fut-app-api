package store

import (
	"context"
	"time"
)

type Notification struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	ChargeID    *string   `json:"charge_id,omitempty"`
	Phone       string    `json:"phone"`
	Message     string    `json:"message"`
	ScheduledAt time.Time `json:"scheduled_at"`
	Status      string    `json:"status"`
}

func (s *Store) ScheduleNotification(ctx context.Context, userID string, chargeID *string, phone, message string, at time.Time) error {
	_, err := s.pool.Exec(ctx, `
		insert into notifications (user_id, charge_id, phone, message, scheduled_at)
		values ($1, $2, $3, $4, $5)`, userID, chargeID, phone, message, at)
	return err
}

// DueNotifications pega lembretes agendados cuja hora chegou e cuja cobrança
// (se houver) ainda está em aberto.
func (s *Store) DueNotifications(ctx context.Context) ([]Notification, error) {
	rows, err := s.pool.Query(ctx, `
		select n.id, n.user_id, n.charge_id, n.phone, n.message, n.scheduled_at, n.status
		from notifications n
		left join charges c on c.id = n.charge_id
		where n.status = 'scheduled' and n.scheduled_at <= now()
		  and (n.charge_id is null or c.status in ('pending', 'overdue'))`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Notification
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.UserID, &n.ChargeID, &n.Phone, &n.Message, &n.ScheduledAt, &n.Status); err != nil {
			return nil, err
		}
		items = append(items, n)
	}
	return items, rows.Err()
}

func (s *Store) MarkNotificationSent(ctx context.Context, id, providerMessageID string) error {
	_, err := s.pool.Exec(ctx,
		`update notifications set status = 'sent', sent_at = now(), provider_message_id = $2 where id = $1`,
		id, providerMessageID)
	return err
}

func (s *Store) MarkNotificationFailed(ctx context.Context, id, errMsg string) error {
	_, err := s.pool.Exec(ctx,
		`update notifications set status = 'failed', error = $2 where id = $1`, id, errMsg)
	return err
}
