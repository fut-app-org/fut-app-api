create table push_subscriptions (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references users(id) on delete cascade,
    endpoint text not null unique,
    p256dh text not null,
    auth text not null,
    updated_at timestamptz not null default now()
);
create index push_subscriptions_user on push_subscriptions(user_id);

create table match_push_deliveries (
    id uuid primary key default gen_random_uuid(),
    subscription_id uuid not null references push_subscriptions(id) on delete cascade,
    match_id uuid not null references matches(id) on delete cascade,
    kind text not null check (kind in ('created', 'reminder')),
    status text not null default 'pending' check (status in ('pending', 'sent', 'failed')),
    attempts int not null default 0,
    next_attempt_at timestamptz not null default now(),
    sent_at timestamptz,
    unique(subscription_id, match_id, kind)
);
create index match_push_due on match_push_deliveries(next_attempt_at) where status = 'pending';
