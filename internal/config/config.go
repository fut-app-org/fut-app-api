// Package config lê a configuração do processo a partir de variáveis de ambiente.
package config

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"os"
)

type Config struct {
	DatabaseURL string
	Port        string
	JWTSecret   []byte
	Env         string // "development" ou "production"
	MediaDir    string

	MercadoPagoAccessToken   string
	MercadoPagoWebhookSecret string

	// Usados só na primeira execução, quando o banco não tem nenhum usuário.
	SeedAdminName     string
	SeedAdminEmail    string
	SeedAdminPassword string
}

func Load() Config {
	cfg := Config{
		DatabaseURL:              getenv("DATABASE_URL", "postgres://futapp:futapp@localhost:5432/futapp?sslmode=disable"),
		Port:                     getenv("PORT", "8080"),
		Env:                      getenv("ENV", "development"),
		MediaDir:                 getenv("MEDIA_DIR", "./data/media"),
		MercadoPagoAccessToken:   os.Getenv("MERCADO_PAGO_ACCESS_TOKEN"),
		MercadoPagoWebhookSecret: os.Getenv("MERCADO_PAGO_WEBHOOK_SECRET"),
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
