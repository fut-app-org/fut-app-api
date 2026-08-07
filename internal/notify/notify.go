// Package notify envia lembretes de WhatsApp.
//
// O envio real usa uma instância Evolution Go (EvolutionSender); sem as
// variáveis EVOLUTION_API_URL/EVOLUTION_API_KEY o LogSender apenas registra a
// mensagem no log, mantendo a fila e o agendamento funcionando de ponta a ponta.
package notify

import "log"

// Sender envia uma mensagem e devolve o id no provedor.
type Sender interface {
	Send(phone, message string) (providerMessageID string, err error)
}

type LogSender struct{}

func (LogSender) Send(phone, message string) (string, error) {
	log.Printf("[whatsapp stub] para %s: %s", phone, message)
	return "log-only", nil
}
