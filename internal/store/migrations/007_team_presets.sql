-- Presets de times: grupos de jogadores salvos pelo admin para agilizar
-- a escalação manual. Membros que não confirmarem presença são ignorados
-- na hora de aplicar o preset.
create table team_presets (
    id         uuid primary key default gen_random_uuid(),
    name       text not null,
    member_ids uuid[] not null default '{}',
    created_by uuid references users(id),
    created_at timestamptz not null default now()
);
