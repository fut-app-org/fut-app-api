-- Template do lembrete automático de cobrança vencida (job diário + fila notifications).
insert into settings (key, value) values
    ('overdue_template', 'Olá, {{nome}}. A mensalidade de {{mes_referencia}}, no valor de {{valor}}, venceu em {{data_vencimento}} e ainda está em aberto. Regularize para evitar a inativação.')
on conflict (key) do nothing;
