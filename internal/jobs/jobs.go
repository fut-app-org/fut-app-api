// Package jobs agenda as rotinas de manutenção do sistema.
package jobs

import (
	"context"
	"fmt"
	"log"

	"github.com/robfig/cron/v3"

	"futdarapaziada/api/internal/notify"
	"futdarapaziada/api/internal/store"
)

type Runner struct {
	store  *store.Store
	sender notify.Sender
	cron   *cron.Cron
}

func New(st *store.Store, sender notify.Sender) *Runner {
	return &Runner{store: st, sender: sender, cron: cron.New()}
}

// Start registra e inicia os jobs. Erros de agenda são bugs de código, então panic.
func (r *Runner) Start() {
	mustAdd := func(spec string, job func()) {
		if _, err := r.cron.AddFunc(spec, job); err != nil {
			panic(fmt.Sprintf("agendando job %q: %v", spec, err))
		}
	}

	mustAdd("* * * * *", r.closeExpiredConfirmations) // a cada minuto
	mustAdd("*/10 * * * *", r.closeExpiredVoting)
	mustAdd("15 0 * * *", r.markOverdueAndInactivate) // diário, 00h15
	mustAdd("*/5 * * * *", r.sendDueNotifications)

	r.cron.Start()
}

func (r *Runner) Stop() { r.cron.Stop() }

func (r *Runner) closeExpiredConfirmations() {
	ctx := context.Background()
	ids, err := r.store.CloseExpiredConfirmations(ctx)
	if err != nil {
		log.Printf("job fechar confirmações: %v", err)
		return
	}
	for range ids {
		r.store.LogActivity(ctx, nil, "confirmations_closed", "Confirmações encerradas automaticamente (prazo atingido)")
	}
}

func (r *Runner) closeExpiredVoting() {
	ctx := context.Background()
	ids, err := r.store.CloseExpiredVoting(ctx)
	if err != nil {
		log.Printf("job fechar votação: %v", err)
		return
	}
	for _, id := range ids {
		results, err := r.store.VoteResults(ctx, id)
		if err != nil {
			continue
		}
		for _, res := range results {
			label := "artilheiro"
			if res.Category == "worst_player" {
				label = "perna de pau"
			}
			for _, winner := range res.Winners {
				r.store.LogActivity(ctx, nil, "vote_result",
					fmt.Sprintf("%s foi eleito %s da partida", winner.Name, label))
			}
		}
	}
}

// markOverdueAndInactivate marca cobranças vencidas e inativa quem estourou o
// prazo de tolerância (overdue_inactivate_days; 0 = inativa no dia seguinte ao
// vencimento).
func (r *Runner) markOverdueAndInactivate() {
	ctx := context.Background()
	if _, err := r.store.MarkOverdueCharges(ctx); err != nil {
		log.Printf("job marcar vencidas: %v", err)
		return
	}

	graceDays := r.store.SettingInt(ctx, "overdue_inactivate_days", 0)
	users, err := r.store.UsersToInactivate(ctx, graceDays)
	if err != nil {
		log.Printf("job inativar inadimplentes: %v", err)
		return
	}
	for _, u := range users {
		err := r.store.ChangeUserStatus(ctx, u.ID, "inactive", "inadimplência: mensalidade vencida", nil)
		if err != nil {
			log.Printf("inativando %s: %v", u.Name, err)
			continue
		}
		r.store.LogActivity(ctx, nil, "user_inactivated",
			fmt.Sprintf("%s foi inativado por inadimplência", u.Name))
	}
}

func (r *Runner) sendDueNotifications() {
	ctx := context.Background()
	due, err := r.store.DueNotifications(ctx)
	if err != nil {
		log.Printf("job lembretes: %v", err)
		return
	}
	for _, n := range due {
		providerID, err := r.sender.Send(n.Phone, n.Message)
		if err != nil {
			_ = r.store.MarkNotificationFailed(ctx, n.ID, err.Error())
			continue
		}
		_ = r.store.MarkNotificationSent(ctx, n.ID, providerID)
	}
}
