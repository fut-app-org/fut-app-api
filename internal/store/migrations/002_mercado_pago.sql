alter table charges
    add column payment_provider text not null default '',
    add column provider_order_id text not null default '',
    add column provider_payment_id text not null default '',
    add column pix_ticket_url text not null default '',
    add column pix_qr_code_base64 text not null default '',
    add column provider_status text not null default '',
    add column provider_status_detail text not null default '';

create unique index idx_charges_provider_order
    on charges (provider_order_id)
    where provider_order_id <> '';
