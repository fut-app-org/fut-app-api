create extension if not exists pgcrypto;

create table users (
    id            uuid primary key default gen_random_uuid(),
    name          text not null,
    email         text not null unique,
    phone         text not null default '',
    password_hash text not null,
    avatar_color  text not null default '#3B82A0',
    role          text not null default 'player' check (role in ('admin', 'player')),
    status        text not null default 'active' check (status in ('active', 'inactive', 'archived')),
    inactive_reason text not null default '',
    created_at    timestamptz not null default now(),
    updated_at    timestamptz not null default now()
);

create table user_status_history (
    id              uuid primary key default gen_random_uuid(),
    user_id         uuid not null references users (id),
    previous_status text not null,
    new_status      text not null,
    reason          text not null default '',
    changed_by      uuid references users (id),
    created_at      timestamptz not null default now()
);

create table invites (
    id           uuid primary key default gen_random_uuid(),
    token        text not null unique,
    invited_name text not null default '',
    role         text not null default 'player' check (role in ('admin', 'player')),
    created_by   uuid not null references users (id),
    expires_at   timestamptz not null,
    used_at      timestamptz,
    used_by      uuid references users (id),
    revoked_at   timestamptz,
    access_count int not null default 0,
    created_at   timestamptz not null default now()
);

create table matches (
    id                    uuid primary key default gen_random_uuid(),
    match_date            date not null,
    start_time            time not null,
    end_time              time not null,
    venue                 text not null,
    address               text not null default '',
    confirmation_deadline timestamptz not null,
    status                text not null default 'open'
        check (status in ('open', 'closed', 'teams_drawn', 'voting', 'finished', 'cancelled')),
    cancel_reason         text not null default '',
    notes                 text not null default '',
    voting_closes_at      timestamptz,
    finished_at           timestamptz,
    finished_by           uuid references users (id),
    created_by            uuid not null references users (id),
    created_at            timestamptz not null default now()
);

create table match_confirmations (
    match_id     uuid not null references matches (id) on delete cascade,
    user_id      uuid not null references users (id),
    response     text not null default 'no_response' check (response in ('going', 'not_going', 'no_response')),
    responded_at timestamptz,
    primary key (match_id, user_id)
);

create table match_teams (
    id         uuid primary key default gen_random_uuid(),
    match_id   uuid not null references matches (id) on delete cascade,
    team_name  text not null,
    team_color text not null,
    position   int not null default 0
);

create table match_team_members (
    team_id uuid not null references match_teams (id) on delete cascade,
    user_id uuid not null references users (id),
    primary key (team_id, user_id)
);

-- Fotografia do rateio no momento da geração: alterações posteriores na base de
-- usuários não mudam cobranças já geradas.
create table charge_batches (
    id                      uuid primary key default gen_random_uuid(),
    reference_month         date not null unique,
    total_amount_cents      bigint not null,
    user_count              int not null,
    individual_amount_cents bigint not null,
    due_date                date not null,
    generated_by            uuid not null references users (id),
    created_at              timestamptz not null default now()
);

create table charges (
    id              uuid primary key default gen_random_uuid(),
    batch_id        uuid not null references charge_batches (id),
    user_id         uuid not null references users (id),
    reference_month date not null,
    amount_cents    bigint not null,
    status          text not null default 'pending'
        check (status in ('pending', 'paid', 'manual_paid', 'overdue', 'cancelled', 'exempt')),
    due_date        date not null,
    paid_at         timestamptz,
    paid_method     text not null default '',
    registered_by   uuid references users (id),
    pix_payload     text not null default '',
    pix_txid        text not null default '',
    created_at      timestamptz not null default now(),
    unique (batch_id, user_id)
);

create table votes (
    id           uuid primary key default gen_random_uuid(),
    match_id     uuid not null references matches (id) on delete cascade,
    voter_id     uuid not null references users (id),
    category     text not null check (category in ('top_scorer', 'worst_player')),
    candidate_id uuid not null references users (id),
    created_at   timestamptz not null default now(),
    unique (match_id, voter_id, category)
);

create table match_media (
    id            uuid primary key default gen_random_uuid(),
    match_id      uuid not null references matches (id) on delete cascade,
    uploaded_by   uuid not null references users (id),
    type          text not null check (type in ('photo', 'video')),
    url           text not null,
    thumbnail_url text not null default '',
    caption       text not null default '',
    status        text not null default 'visible' check (status in ('visible', 'removed')),
    created_at    timestamptz not null default now()
);

create table notifications (
    id                  uuid primary key default gen_random_uuid(),
    user_id             uuid not null references users (id),
    charge_id           uuid references charges (id),
    channel             text not null default 'whatsapp',
    phone               text not null,
    message             text not null,
    scheduled_at        timestamptz not null,
    sent_at             timestamptz,
    status              text not null default 'scheduled' check (status in ('scheduled', 'sent', 'failed')),
    provider_message_id text not null default '',
    error               text not null default '',
    created_at          timestamptz not null default now()
);

create table activity_log (
    id         uuid primary key default gen_random_uuid(),
    actor_id   uuid references users (id),
    kind       text not null,
    message    text not null,
    created_at timestamptz not null default now()
);

create table settings (
    key   text primary key,
    value text not null
);

insert into settings (key, value) values
    ('monthly_total_cents', '0'),
    ('overdue_inactivate_days', '0'),
    ('voting_close_days', '2'),
    ('invite_valid_days', '7'),
    ('reminder_template', 'Olá, {{nome}}. A mensalidade de {{mes_referencia}}, no valor de {{valor}}, ainda está pendente. O prazo para pagamento termina em {{data_vencimento}}.');

create index idx_charges_user on charges (user_id, reference_month desc);
create index idx_charges_status on charges (status, due_date);
create index idx_matches_date on matches (match_date desc);
create index idx_activity_created on activity_log (created_at desc);
create index idx_notifications_due on notifications (status, scheduled_at);
