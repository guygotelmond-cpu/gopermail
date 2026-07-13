// Command smtpd runs the mail ingestion service. It owns nothing but the
// SMTP protocol surface: accepting mail, evaluating filter rules, and
// persisting the result. Rule *management* lives in controld/apiserver.
package main

import (
	"log"
	"os"
	"time"

	"gomail.com/db"
	"gomail.com/smtp"
)

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	pgConn := getenv("POSTGRES_DSN", "postgres://smtp_user:smtp_password@127.0.0.1:5432/smtp_metadata?sslmode=disable")
	mongoURI := getenv("MONGO_URI", "mongodb://127.0.0.1:27017")
	addr := getenv("SMTP_ADDR", "0.0.0.0:1025")
	domain := getenv("SMTP_DOMAIN", "localhost")

	// 1. Initialize Postgres and catch errors explicitly
	rStore, err := db.NewPostgresService(pgConn)
	if err != nil {
		log.Fatalf("CRITICAL: Postgres connection failure (Is Docker running?): %v", err)
	}
	defer rStore.Close()

	// 2. Initialize MongoDB and catch errors explicitly
	dStore, err := db.NewMongoService(mongoURI, "mail_vault", "payloads")
	if err != nil {
		log.Fatalf("CRITICAL: MongoDB connection failure (Is Docker running?): %v", err)
	}
	defer dStore.Close()

	// 3. Inject initialized handlers into the backend
	backend := &smtp.Backend{
		RStore: rStore,
		DStore: dStore,
	}

	config := smtp.Config{
		Addr:            addr,
		Domain:          domain,
		MaxMessageBytes: 25 * 1024 * 1024,
		MaxRecipients:   100,
		ReadTimeout:     3 * time.Minute,
		WriteTimeout:    3 * time.Minute,
		EnableTLS:       false,
		AllowInsecure:   true,
	}

	server, err := smtp.NewServer(backend, config)
	if err != nil {
		log.Fatalf("Initialization abort: %v", err)
	}

	log.Printf("[smtpd] Starting Database-Backed SMTP server on %s...", config.Addr)
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Server crash: %v", err)
	}
}
