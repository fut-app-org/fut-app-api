package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"futdarapaziada/api/internal/push"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Each test uses a separate schema; no application records are read or modified.
func pushTestStore(t *testing.T) *Store {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	root, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	schema := "push_test_" + uuid.NewString()[:8]
	if _, err = root.Exec(ctx, `create schema `+schema); err != nil {
		root.Close()
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	st := &Store{pool: pool}
	t.Cleanup(func() {
		pool.Close()
		_, err := root.Exec(ctx, `drop schema `+schema+` cascade`)
		root.Close()
		if err != nil {
			t.Error(err)
		}
	})
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	return st
}
func pushTestUser(t *testing.T, s *Store, status string) (string, push.Subscription) {
	t.Helper()
	ctx := context.Background()
	var id string
	err := s.pool.QueryRow(ctx, `insert into users(name,email,password_hash,status) values('Push Test',$1,'test',$2) returning id`, uuid.NewString()+"@example.invalid", status).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	sub := push.Subscription{Endpoint: "https://fcm.googleapis.com/" + uuid.NewString()}
	sub.Keys.Auth = "test-auth"
	sub.Keys.P256dh = "test-key"
	if err := s.SavePushSubscription(ctx, id, sub); err != nil {
		t.Fatal(err)
	}
	return id, sub
}
func pushTestMatch(t *testing.T, s *Store, creator string) Match {
	t.Helper()
	m, err := s.CreateMatch(context.Background(), MatchInput{MatchDate: "2099-01-01", StartTime: "20:00", EndTime: "22:00", Venue: "Test venue", ConfirmationDeadline: time.Now().Add(time.Hour)}, creator)
	if err != nil {
		t.Fatal(err)
	}
	return m
}
func TestPushCreationAndDelivery(t *testing.T) {
	s := pushTestStore(t)
	ctx := context.Background()
	active, sub := pushTestUser(t, s, "active")
	pushTestUser(t, s, "inactive")
	m := pushTestMatch(t, s, active)
	d, err := s.ClaimPushDelivery(ctx, 2*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if d.MatchID != m.ID || d.Kind != "created" || d.Subscription.Endpoint != sub.Endpoint {
		t.Fatalf("wrong recipient/event: %+v", d)
	}
	if _, err := s.ClaimPushDelivery(ctx, 2*time.Hour); !errors.Is(err, ErrNotFound) {
		t.Fatalf("duplicate or inactive delivery: %v", err)
	}
	if err := s.CompletePushDelivery(ctx, d, 201, nil); err != nil {
		t.Fatal(err)
	}
	var state string
	s.pool.QueryRow(ctx, `select status from match_push_deliveries where id=$1`, d.ID).Scan(&state)
	if state != "sent" {
		t.Fatal(state)
	}
}
func TestPushRemindersRespectResponsesAndDeadline(t *testing.T) {
	s := pushTestStore(t)
	ctx := context.Background()
	user, _ := pushTestUser(t, s, "active")
	m := pushTestMatch(t, s, user)
	if _, err := s.pool.Exec(ctx, `update matches set created_at=now()-interval '6 minutes' where id=$1`, m.ID); err != nil {
		t.Fatal(err)
	}
	created, err := s.ClaimPushDelivery(ctx, 2*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.CompletePushDelivery(ctx, created, 201, nil); err != nil {
		t.Fatal(err)
	}
	if err = s.QueueMatchReminders(ctx, 2*time.Hour); err != nil {
		t.Fatal(err)
	}
	if err = s.QueueMatchReminders(ctx, 2*time.Hour); err != nil {
		t.Fatal(err)
	}
	var count int
	s.pool.QueryRow(ctx, `select count(*) from match_push_deliveries where kind='reminder'`).Scan(&count)
	if count != 1 {
		t.Fatalf("reminders: %d", count)
	}
	if err = s.Confirm(ctx, m.ID, user, "going"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ClaimPushDelivery(ctx, 2*time.Hour); !errors.Is(err, ErrNotFound) {
		t.Fatalf("responded user got reminder: %v", err)
	}
	if err = s.Confirm(ctx, m.ID, user, "no_response"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.pool.Exec(ctx, `update matches set confirmation_deadline=now()+interval '1 day' where id=$1`, m.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ClaimPushDelivery(ctx, 2*time.Hour); !errors.Is(err, ErrNotFound) {
		t.Fatalf("old deadline got reminder: %v", err)
	}
	if _, err = s.pool.Exec(ctx, `update matches set confirmation_deadline=now()+interval '1 hour' where id=$1`, m.ID); err != nil {
		t.Fatal(err)
	}
	d, err := s.ClaimPushDelivery(ctx, 2*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if d.Kind != "reminder" {
		t.Fatal(d.Kind)
	}
}
func TestPushRetryAndExpiredSubscription(t *testing.T) {
	s := pushTestStore(t)
	ctx := context.Background()
	user, sub := pushTestUser(t, s, "active")
	pushTestMatch(t, s, user)
	d, err := s.ClaimPushDelivery(ctx, 2*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.CompletePushDelivery(ctx, d, 503, fmt.Errorf("unavailable")); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ClaimPushDelivery(ctx, 2*time.Hour); !errors.Is(err, ErrNotFound) {
		t.Fatal("retry has no backoff")
	}
	if _, err = s.pool.Exec(ctx, `update match_push_deliveries set next_attempt_at=now() where id=$1`, d.ID); err != nil {
		t.Fatal(err)
	}
	retry, err := s.ClaimPushDelivery(ctx, 2*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if retry.Attempts != 2 {
		t.Fatal(retry.Attempts)
	}
	if err = s.CompletePushDelivery(ctx, retry, 410, fmt.Errorf("gone")); err != nil {
		t.Fatal(err)
	}
	exists, err := s.HasPushSubscription(ctx, user, sub.Endpoint)
	if err != nil || exists {
		t.Fatalf("expired endpoint retained: %v", err)
	}
}
func TestPushCancellationAndOwnership(t *testing.T) {
	s := pushTestStore(t)
	ctx := context.Background()
	user, sub := pushTestUser(t, s, "active")
	other, _ := pushTestUser(t, s, "inactive")
	if err := s.DeletePushSubscription(ctx, other, sub.Endpoint); err != nil {
		t.Fatal(err)
	}
	exists, err := s.HasPushSubscription(ctx, user, sub.Endpoint)
	if err != nil || !exists {
		t.Fatal("another user removed subscription")
	}
	m := pushTestMatch(t, s, user)
	if _, err = s.pool.Exec(ctx, `update matches set status='cancelled' where id=$1`, m.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ClaimPushDelivery(ctx, 2*time.Hour); !errors.Is(err, ErrNotFound) {
		t.Fatal("cancelled match delivered")
	}
	if err = s.DeletePushSubscription(ctx, user, sub.Endpoint); err != nil {
		t.Fatal(err)
	}
	exists, err = s.HasPushSubscription(ctx, user, sub.Endpoint)
	if err != nil || exists {
		t.Fatal("unsubscribe failed")
	}
}

func TestSharedDeviceDoesNotInheritOtherAccountsQueue(t *testing.T) {
	s := pushTestStore(t)
	ctx := context.Background()
	first, sub := pushTestUser(t, s, "active")
	other, _ := pushTestUser(t, s, "inactive")
	pushTestMatch(t, s, first)
	if err := s.SavePushSubscription(ctx, other, sub); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.pool.QueryRow(ctx, `select count(*) from match_push_deliveries`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("shared device inherited queue from previous account")
	}
}
