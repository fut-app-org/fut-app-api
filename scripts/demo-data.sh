#!/usr/bin/env bash

set -euo pipefail

readonly PROJECT_DIR="${FUT_APP_API_DIR:-/opt/fut-app-api}"
readonly COMPOSE_FILE="$PROJECT_DIR/compose.yaml"
readonly ENV_FILE="$PROJECT_DIR/.env"

usage() {
  echo "Uso: bash $0 seed|reset" >&2
  exit 2
}

if [[ $# -ne 1 ]]; then
  usage
fi

if [[ ! -f "$COMPOSE_FILE" || ! -f "$ENV_FILE" ]]; then
  echo "Projeto ou .env não encontrados em $PROJECT_DIR." >&2
  echo "Defina FUT_APP_API_DIR caso o projeto esteja em outro caminho." >&2
  exit 1
fi

psql_in_postgres() {
  docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" exec -T postgres \
    sh -c 'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"'
}

case "$1" in
seed)
  psql_in_postgres <<'SQL'
begin;

do $$
declare
  seed_key constant text := 'demo_seed_fut_app_v1';
  marker constant text := '[DADOS_DEMO_FUT_APP_V1]';
  admin_id uuid;
  active_user_count integer;
  current_batch_id uuid;
  previous_batch_id uuid;
  open_match_id uuid;
  voting_match_id uuid;
  finished_match_id uuid;
  green_team_id uuid;
  lime_team_id uuid;
  top_scorer_id uuid;
  worst_player_id uuid;
  current_month date := date_trunc('month', current_date)::date;
  previous_month date := (date_trunc('month', current_date) - interval '1 month')::date;
  monthly_total_cents bigint := 30000;
  individual_amount_cents bigint;
begin
  if exists (select 1 from settings where key = seed_key) then
    raise exception 'O seed de demonstração já está aplicado. Execute primeiro: bash scripts/demo-data.sh reset';
  end if;

  select id into admin_id
  from users
  where role = 'admin' and status = 'active'
  order by created_at
  limit 1;

  if admin_id is null then
    raise exception 'Nenhum administrador ativo foi encontrado. O seed não alterou nada.';
  end if;

  select count(*) into active_user_count from users where status = 'active';
  if active_user_count = 0 then
    raise exception 'Nenhum usuário ativo foi encontrado. O seed não alterou nada.';
  end if;

  insert into matches (
    match_date, start_time, end_time, venue, address, confirmation_deadline, notes, created_by
  ) values (
    current_date + 7, '19:00', '21:00', 'Arena da Rapaziada',
    'Rua do Futebol, 10', now() + interval '4 days',
    marker || ' Partida aberta para testar confirmação de presença.', admin_id
  ) returning id into open_match_id;

  insert into match_confirmations (match_id, user_id)
  select open_match_id, id from users where status = 'active';

  with ranked_users as (
    select id, row_number() over (order by name, id) as position
    from users
    where status = 'active'
  )
  update match_confirmations confirmation
  set response = case
        when ranked_users.position % 3 = 1 then 'going'
        when ranked_users.position % 3 = 2 then 'not_going'
        else 'no_response'
      end,
      responded_at = case
        when ranked_users.position % 3 = 0 then null
        else now() - interval '2 hours'
      end
  from ranked_users
  where confirmation.match_id = open_match_id and confirmation.user_id = ranked_users.id;

  insert into matches (
    match_date, start_time, end_time, venue, address, confirmation_deadline, notes,
    status, voting_closes_at, finished_at, finished_by, created_by
  ) values (
    current_date - 1, '20:00', '22:00', 'Quadra Central',
    'Avenida do Esporte, 500', now() - interval '2 days',
    marker || ' Partida em votação para testar os votos dos jogadores.',
    'voting', now() + interval '1 day', now() - interval '3 hours', admin_id, admin_id
  ) returning id into voting_match_id;

  insert into match_confirmations (match_id, user_id, response, responded_at)
  select voting_match_id, id, 'going', now() - interval '1 day'
  from users where status = 'active';

  insert into matches (
    match_date, start_time, end_time, venue, address, confirmation_deadline, notes,
    status, voting_closes_at, finished_at, finished_by, created_by
  ) values (
    current_date - 14, '19:30', '21:30', 'Campo do Bairro',
    'Praça das Chuteiras, 25', now() - interval '16 days',
    marker || ' Partida finalizada com times e resultado de votação.',
    'finished', now() - interval '10 days', now() - interval '13 days', admin_id, admin_id
  ) returning id into finished_match_id;

  insert into match_confirmations (match_id, user_id, response, responded_at)
  select finished_match_id, id, 'going', now() - interval '14 days'
  from users where status = 'active';

  insert into match_teams (match_id, team_name, team_color, position)
  values (finished_match_id, 'Time Verde', '#0A3B28', 1)
  returning id into green_team_id;

  insert into match_teams (match_id, team_name, team_color, position)
  values (finished_match_id, 'Time Lima', '#C8F14B', 2)
  returning id into lime_team_id;

  with ranked_users as (
    select id, row_number() over (order by name, id) as position
    from users
    where status = 'active'
  )
  insert into match_team_members (team_id, user_id)
  select case when position % 2 = 1 then green_team_id else lime_team_id end, id
  from ranked_users;

  select id into top_scorer_id
  from users where status = 'active' order by name, id limit 1;

  select id into worst_player_id
  from users where status = 'active' order by name desc, id desc limit 1;

  insert into votes (match_id, voter_id, category, candidate_id)
  select finished_match_id, id, 'top_scorer', top_scorer_id
  from users where status = 'active';

  insert into votes (match_id, voter_id, category, candidate_id)
  select finished_match_id, id, 'worst_player', worst_player_id
  from users where status = 'active';

  individual_amount_cents := monthly_total_cents / active_user_count;

  if not exists (select 1 from charge_batches where reference_month = current_month) then
    insert into charge_batches (
      reference_month, total_amount_cents, user_count, individual_amount_cents, due_date, generated_by
    ) values (
      current_month, monthly_total_cents, active_user_count, individual_amount_cents,
      current_date + 10, admin_id
    ) returning id into current_batch_id;

    with ranked_users as (
      select id, row_number() over (order by name, id) as position
      from users where status = 'active'
    )
    insert into charges (
      batch_id, user_id, reference_month, amount_cents, due_date, status, paid_at,
      paid_method, registered_by, pix_txid
    )
    select current_batch_id, id, current_month, individual_amount_cents, current_date + 10,
      case when position % 2 = 1 then 'manual_paid' else 'pending' end,
      case when position % 2 = 1 then now() - interval '1 day' else null end,
      case when position % 2 = 1 then 'pix' else '' end,
      case when position % 2 = 1 then admin_id else null end,
      marker
    from ranked_users;
  end if;

  if not exists (select 1 from charge_batches where reference_month = previous_month) then
    insert into charge_batches (
      reference_month, total_amount_cents, user_count, individual_amount_cents, due_date, generated_by
    ) values (
      previous_month, monthly_total_cents, active_user_count, individual_amount_cents,
      previous_month + interval '10 days', admin_id
    ) returning id into previous_batch_id;

    insert into charges (
      batch_id, user_id, reference_month, amount_cents, due_date, status, paid_at,
      paid_method, registered_by, pix_txid
    )
    select previous_batch_id, id, previous_month, individual_amount_cents,
      previous_month + interval '10 days', 'manual_paid', previous_month + interval '8 days',
      'pix', admin_id, marker
    from users where status = 'active';
  end if;

  insert into activity_log (actor_id, kind, message) values
    (admin_id, 'demo_seed', marker || ' Dados de demonstração criados para revisão visual.'),
    (admin_id, 'demo_seed', marker || ' Nenhum usuário, senha ou perfil foi alterado.');

  insert into settings (key, value)
  values (
    seed_key,
    jsonb_build_object(
      'marker', marker,
      'current_batch_id', current_batch_id,
      'previous_batch_id', previous_batch_id,
      'seeded_at', now()
    )::text
  );
end $$;

commit;

select 'Seed aplicado. Usuários não foram criados nem alterados.' as resultado;
SQL
  ;;
reset)
  psql_in_postgres <<'SQL'
begin;

do $$
declare
  seed_key constant text := 'demo_seed_fut_app_v1';
  config jsonb;
  marker text;
  current_batch_id uuid;
  previous_batch_id uuid;
begin
  select value::jsonb into config from settings where key = seed_key for update;
  if config is null then
    raise exception 'Nenhum seed de demonstração está aplicado.';
  end if;

  marker := config->>'marker';
  current_batch_id := nullif(config->>'current_batch_id', '')::uuid;
  previous_batch_id := nullif(config->>'previous_batch_id', '')::uuid;

  delete from notifications
  where charge_id in (select id from charges where pix_txid = marker);

  delete from charges where pix_txid = marker;

  delete from charge_batches
  where id in (current_batch_id, previous_batch_id)
    and not exists (select 1 from charges where charges.batch_id = charge_batches.id);

  delete from matches where notes like '%' || marker || '%';
  delete from activity_log where message like '%' || marker || '%';
  delete from settings where key = seed_key;
end $$;

commit;

select 'Seed removido. Apenas dados identificados pelo marcador foram excluídos.' as resultado;
SQL
  ;;
*)
  usage
  ;;
esac
