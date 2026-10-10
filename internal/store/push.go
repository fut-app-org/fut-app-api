package store

import (
	"context"
	"futdarapaziada/api/internal/push"
	"github.com/jackc/pgx/v5"
	"time"
)

func (s *Store) SavePushSubscription(ctx context.Context, userID string, sub push.Subscription) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	// Reassigning a shared device must not inherit another account's queue.
	_, err = tx.Exec(ctx, `delete from match_push_deliveries d using push_subscriptions p
 where d.subscription_id=p.id and p.endpoint=$1 and p.user_id<>$2`, sub.Endpoint, userID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `insert into push_subscriptions(user_id,endpoint,p256dh,auth) values($1,$2,$3,$4)
 on conflict(endpoint) do update set user_id=excluded.user_id,p256dh=excluded.p256dh,auth=excluded.auth,updated_at=now()`, userID, sub.Endpoint, sub.Keys.P256dh, sub.Keys.Auth)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Store) DeletePushSubscription(ctx context.Context, userID, endpoint string) error {
	_, err := s.pool.Exec(ctx, `delete from push_subscriptions where user_id=$1 and endpoint=$2`, userID, endpoint)
	return err
}
func (s *Store) HasPushSubscription(ctx context.Context, userID, endpoint string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `select exists(select 1 from push_subscriptions where user_id=$1 and endpoint=$2)`, userID, endpoint).Scan(&exists)
	return exists, err
}

// Schedule reminders using the current deadline. Unique rows prevent repeat sends.
func (s *Store) QueueMatchReminders(ctx context.Context, lead time.Duration) error {
	if lead <= 0 {
		return nil
	}
	_, err := s.pool.Exec(ctx, `insert into match_push_deliveries(subscription_id,match_id,kind)
 select p.id,m.id,'reminder' from matches m
 join match_confirmations c on c.match_id=m.id and c.response='no_response'
 join users u on u.id=c.user_id and u.status='active'
 join push_subscriptions p on p.user_id=u.id
 where m.status='open' and m.confirmation_deadline>now()
 and m.confirmation_deadline<=now()+($1*interval '1 second')
 and m.created_at<now()-interval '5 minutes'
 on conflict do nothing`, lead.Seconds())
	return err
}

type PushDelivery struct {
	ID, SubscriptionID, MatchID, Kind, MatchDate, StartTime, Venue string
	Subscription                                                   push.Subscription
	Deadline                                                       time.Time
	Attempts                                                       int
}

// A short lease protects against concurrent workers and recovers after a crash.
func (s *Store) ClaimPushDelivery(ctx context.Context, lead time.Duration) (PushDelivery, error) {
	var d PushDelivery
	err := s.pool.QueryRow(ctx, `with candidate as (
 select d.id from match_push_deliveries d
 join push_subscriptions p on p.id=d.subscription_id
 join users u on u.id=p.user_id
 join matches m on m.id=d.match_id
 where d.status='pending' and d.next_attempt_at<=now() and d.attempts<5
 and u.status='active' and m.status='open' and m.confirmation_deadline>now()
 and (d.kind='created' or ($1>0 and m.confirmation_deadline<=now()+($1*interval '1 second')
 and exists(select 1 from match_confirmations c where c.match_id=m.id and c.user_id=u.id and c.response='no_response')))
 order by d.next_attempt_at,d.id for update of d skip locked limit 1
 ), claimed as (
 update match_push_deliveries d set attempts=attempts+1,next_attempt_at=now()+interval '2 minutes'
 from candidate c where d.id=c.id returning d.*
 ) select d.id,d.subscription_id,d.match_id,d.kind,d.attempts,p.endpoint,p.p256dh,p.auth,
 m.match_date::text,to_char(m.start_time,'HH24:MI'),m.venue,m.confirmation_deadline
 from claimed d join push_subscriptions p on p.id=d.subscription_id join matches m on m.id=d.match_id`, lead.Seconds()).Scan(
		&d.ID, &d.SubscriptionID, &d.MatchID, &d.Kind, &d.Attempts, &d.Subscription.Endpoint, &d.Subscription.Keys.P256dh, &d.Subscription.Keys.Auth, &d.MatchDate, &d.StartTime, &d.Venue, &d.Deadline)
	if err == pgx.ErrNoRows {
		return d, ErrNotFound
	}
	return d, err
}
func (s *Store) CompletePushDelivery(ctx context.Context, d PushDelivery, status int, sendErr error) error {
	if status == 404 || status == 410 {
		_, err := s.pool.Exec(ctx, `delete from push_subscriptions where id=$1 and endpoint=$2`, d.SubscriptionID, d.Subscription.Endpoint)
		return err
	}
	if sendErr == nil {
		_, err := s.pool.Exec(ctx, `update match_push_deliveries set status='sent',sent_at=now() where id=$1`, d.ID)
		return err
	}
	retry := status == 0 || status == 429 || status >= 500
	state := "pending"
	if !retry || d.Attempts >= 5 {
		state = "failed"
	}
	_, err := s.pool.Exec(ctx, `update match_push_deliveries set status=$2,next_attempt_at=now()+($3*interval '1 second') where id=$1`, d.ID, state, 60*(1<<d.Attempts))
	return err
}
