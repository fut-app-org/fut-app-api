// Package jobs agenda as rotinas de manutenção do sistema.
package jobs

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

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

// markOverdueAndInactivate marca cobranças vencidas, agenda o lembrete de
// WhatsApp para cada uma e inativa quem estourou o prazo de tolerância
// (overdue_inactivate_days; 0 = inativa no dia seguinte ao vencimento).
func (r *Runner) markOverdueAndInactivate() {
	ctx := context.Background()
	overdue, err := r.store.MarkOverdueCharges(ctx)
	if err != nil {
		log.Printf("job marcar vencidas: %v", err)
		return
	}
	r.scheduleOverdueReminders(ctx, overdue)

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

const defaultOverdueTemplate = "Olá, {{nome}}. A mensalidade de {{mes_referencia}}, no valor de {{valor}}, venceu em {{data_vencimento}} e ainda está em aberto. Regularize para evitar a inativação."

// scheduleOverdueReminders agenda o lembrete de vencido de cada cobrança que
// acabou de transicionar — a fila (sendDueNotifications) faz o envio.
func (r *Runner) scheduleOverdueReminders(ctx context.Context, overdue []store.OverdueCharge) {
	if len(overdue) == 0 {
		return
	}
	template := r.store.SettingString(ctx, "overdue_template", defaultOverdueTemplate)
	for _, o := range overdue {
		phone, err := notify.NormalizeNumber(o.UserPhone)
		if err != nil {
			log.Printf("lembrete de vencido: %s sem WhatsApp válido (%q): %v", o.UserName, o.UserPhone, err)
			continue
		}
		dueDate, err := time.Parse("2006-01-02", o.DueDate)
		if err != nil {
			log.Printf("lembrete de vencido: vencimento inválido na cobrança %s: %v", o.ID, err)
			continue
		}
		message := renderTemplate(template, map[string]string{
			"{{nome}}":            o.UserName,
			"{{mes_referencia}}":  o.ReferenceMonth,
			"{{valor}}":           formatCentsBR(o.AmountCents),
			"{{data_vencimento}}": dueDate.Format("02/01/2006"),
		})
		if _, err := r.store.ScheduleNotification(ctx, o.UserID, &o.ID, phone, message, time.Now()); err != nil {
			log.Printf("agendando lembrete de vencido para %s: %v", o.UserName, err)
		}
	}
}

func renderTemplate(template string, replacements map[string]string) string {
	for placeholder, value := range replacements {
		template = strings.ReplaceAll(template, placeholder, value)
	}
	return template
}

// formatCentsBR formata centavos como "R$ 80,00".
func formatCentsBR(cents int64) string {
	return fmt.Sprintf("R$ %d,%02d", cents/100, cents%100)
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
