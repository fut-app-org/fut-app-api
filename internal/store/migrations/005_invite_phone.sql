-- Telefone opcional do convidado: permite enviar o convite pelo Evolution Go
-- em vez de abrir o compartilhamento do WhatsApp no navegador do admin.
alter table invites add column if not exists phone text not null default '';
