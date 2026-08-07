// Comando api sobe o servidor HTTP do Fut da Rapaziada.
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"os"

	"futdarapaziada/api/internal/api"
	"futdarapaziada/api/internal/auth"
	"futdarapaziada/api/internal/config"
	"futdarapaziada/api/internal/jobs"
	"futdarapaziada/api/internal/notify"
	"futdarapaziada/api/internal/store"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()

	st, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()

	if err := st.Migrate(ctx); err != nil {
		log.Fatalf("aplicando migrations: %v", err)
	}
	if err := seedAdmin(ctx, cfg, st); err != nil {
		log.Fatalf("criando admin inicial: %v", err)
	}
	if err := os.MkdirAll(cfg.MediaDir, 0o755); err != nil {
		log.Fatalf("criando diretório de mídia: %v", err)
	}

	sender := notify.Sender(notify.LogSender{})
	if cfg.EvolutionAPIURL != "" && cfg.EvolutionAPIKey != "" {
		sender = notify.NewEvolutionSender(cfg.EvolutionAPIURL, cfg.EvolutionAPIKey)
		log.Printf("WhatsApp via Evolution Go em %s", cfg.EvolutionAPIURL)
	} else {
		log.Println("EVOLUTION_API_URL/EVOLUTION_API_KEY não definidos; WhatsApp apenas em log")
	}

	runner := jobs.New(st, sender)
	runner.Start()
	defer runner.Stop()

	server := api.NewServer(cfg, st, sender)
	log.Printf("API ouvindo em :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, server.Handler()); err != nil {
		log.Fatal(err)
	}
}

// seedAdmin cria o primeiro administrador quando o banco está vazio, para que
// exista alguém capaz de gerar convites.
func seedAdmin(ctx context.Context, cfg config.Config, st *store.Store) error {
	count, err := st.CountUsers(ctx)
	if err != nil || count > 0 {
		return err
	}

	password := cfg.SeedAdminPassword
	if password == "" {
		for {
			buf := make([]byte, 9)
			if _, err := rand.Read(buf); err != nil {
				return err
			}
			password = base64.RawURLEncoding.EncodeToString(buf)
			if auth.ValidatePassword(password) == nil {
				break
			}
		}
	} else if err := auth.ValidatePassword(password); err != nil {
		return fmt.Errorf("senha inicial do administrador: %w", err)
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	user, err := st.CreateUser(ctx, cfg.SeedAdminName, cfg.SeedAdminEmail, "", hash, "#0A3B28", "admin")
	if err != nil {
		return err
	}
	log.Printf("admin inicial criado: %s", user.Email)
	if cfg.SeedAdminPassword == "" {
		log.Printf("senha gerada do admin (troque após o primeiro login): %s", password)
	}
	return nil
}
