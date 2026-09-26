-- Cada participante passa a poder votar em até 2 jogadores por categoria.
-- A unicidade sai de (partida, votante, categoria) para incluir o candidato:
-- impede voto duplicado no mesmo jogador e permite 2 votos distintos.
-- O limite de 2 por categoria é validado na aplicação (handleVote).
alter table votes drop constraint votes_match_id_voter_id_category_key;

alter table votes
    add constraint votes_match_voter_category_candidate_key
    unique (match_id, voter_id, category, candidate_id);
