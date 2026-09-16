-- Lotes de cobrança passam a ter um tipo: 'monthly' (mensalidade, única por mês)
-- ou 'match' (partida avulsa, ilimitada por mês, com participantes escolhidos).
alter table charge_batches
    add column kind text not null default 'monthly' check (kind in ('monthly', 'match')),
    add column title text not null default '';

-- A unicidade por mês vale apenas para mensalidades.
alter table charge_batches drop constraint charge_batches_reference_month_key;
create unique index charge_batches_monthly_month
    on charge_batches (reference_month)
    where kind = 'monthly';
