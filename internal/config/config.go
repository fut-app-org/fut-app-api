// Package config lê a configuração do processo a partir de variáveis de ambiente.
package config

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"os"
	"strconv"
)

type Config struct {
	DatabaseURL string
	Port        string
	JWTSecret   []byte
	Env         string // "development" ou "production"
	MediaDir    string
	DisableJobs bool // desativa o cron em instâncias locais que compartilham o banco

	VAPIDPublicKey     string
	VAPIDPrivateKey    string
	VAPIDSubject       string
	MatchReminderHours int

	MercadoPagoAccessToken   string
	MercadoPagoWebhookSecret string
	MercadoPagoTestMode      bool
	ResendAPIKey             string
	EmailFrom                string
	AppURL                   string

	// Evolution Go (WhatsApp). Sem URL+key o envio cai no LogSender (só loga).
	EvolutionAPIURL string
	EvolutionAPIKey string

	// Usados só na primeira execução, quando o banco não tem nenhum usuário.
	SeedAdminName     string
	SeedAdminEmail    string
	SeedAdminPassword string
}

func Load() Config {
	cfg := Config{
		VAPIDPublicKey:           os.Getenv("VAPID_PUBLIC_KEY"),
		VAPIDPrivateKey:          os.Getenv("VAPID_PRIVATE_KEY"),
		VAPIDSubject:             os.Getenv("VAPID_SUBJECT"),
		MatchReminderHours:       reminderHours(),
		DatabaseURL:              getenv("DATABASE_URL", "postgres://futapp:futapp@localhost:5432/futapp?sslmode=disable"),
		Port:                     getenv("PORT", "8080"),
		Env:                      getenv("ENV", "development"),
		MediaDir:                 getenv("MEDIA_DIR", "./data/media"),
		DisableJobs:              getenvBool("DISABLE_JOBS"),
		MercadoPagoAccessToken:   os.Getenv("MERCADO_PAGO_ACCESS_TOKEN"),
		MercadoPagoWebhookSecret: os.Getenv("MERCADO_PAGO_WEBHOOK_SECRET"),
		MercadoPagoTestMode:      getenvBool("MERCADO_PAGO_TEST_MODE"),
		ResendAPIKey:             os.Getenv("RESEND_API_KEY"),
		EmailFrom:                os.Getenv("EMAIL_FROM"),
		AppURL:                   getenv("APP_URL", "http://localhost:5173"),
		EvolutionAPIURL:          os.Getenv("EVOLUTION_API_URL"),
		EvolutionAPIKey:          os.Getenv("EVOLUTION_API_KEY"),
		SeedAdminName:            getenv("SEED_ADMIN_NAME", "Administrador"),
		SeedAdminEmail:           getenv("SEED_ADMIN_EMAIL", "admin@futdarapaziada.local"),
		SeedAdminPassword:        os.Getenv("SEED_ADMIN_PASSWORD"),
	}

	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		// Sem segredo fixo as sessões caem a cada restart — aceitável só em dev.
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			log.Fatalf("gerando JWT_SECRET aleatório: %v", err)
		}
		secret = hex.EncodeToString(buf)
		log.Println("aviso: JWT_SECRET não definido; usando segredo aleatório (sessões não sobrevivem a restart)")
	}
	cfg.JWTSecret = []byte(secret)
	return cfg
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvBool(key string) bool {
	value, err := strconv.ParseBool(os.Getenv(key))
	return err == nil && value
}

func reminderHours() int {
	value, err := strconv.Atoi(getenv("MATCH_REMINDER_HOURS", "2"))
	if err != nil || value < 0 || value > 168 {
		return 2
	}
	return value
}
