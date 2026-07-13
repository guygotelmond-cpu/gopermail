// Command apiserver is the browser-facing REST gateway. Reads (rules, email
// log, auth) go straight to Postgres; writes to rules are proxied through
// controld's gRPC AddRule so rule mutation stays centralized in one place.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"time"

	"gomail.com/db"
	pb "gomail.com/proto/control"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

type api struct {
	rStore     *db.PostgresService
	dStore     *db.MongoService
	ruleClient pb.RuleServiceClient
	smtpAddr   string
}

func withCORS(allowOrigin string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", allowOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

// --- Auth ---

type authRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (a *api) handleSignup(w http.ResponseWriter, r *http.Request) {
	var req authRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := a.rStore.CreateUser(req.Username, req.Password); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "created"})
}

func (a *api) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req authRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := a.rStore.Authenticate(req.Username, req.Password); err != nil {
		writeErr(w, http.StatusUnauthorized, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// --- Rules ---

type ruleDTO struct {
	ID          int    `json:"id"`
	Field       string `json:"field"`
	Operator    string `json:"operator"`
	Value       string `json:"value"`
	Action      string `json:"action"`
	ActionValue string `json:"action_value"`
}

func (a *api) handleListRules(w http.ResponseWriter, r *http.Request) {
	username := r.URL.Query().Get("username")
	if username == "" {
		writeErr(w, http.StatusBadRequest, errAny("username query param is required"))
		return
	}
	rules, err := a.rStore.GetRulesForUser(username)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	out := make([]ruleDTO, 0, len(rules))
	for _, rl := range rules {
		out = append(out, ruleDTO{ID: rl.ID, Field: rl.Field, Operator: rl.Operator, Value: rl.Value, Action: rl.Action, ActionValue: rl.ActionValue})
	}
	writeJSON(w, http.StatusOK, out)
}

type addRuleRequest struct {
	Username    string `json:"username"`
	Field       string `json:"field"`
	Operator    string `json:"operator"`
	Value       string `json:"value"`
	Action      string `json:"action"`
	ActionValue string `json:"action_value"`
}

func (a *api) handleAddRule(w http.ResponseWriter, r *http.Request) {
	var req addRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Delegate the actual write to controld over gRPC, keeping rule mutation
	// logic in one place.
	resp, err := a.ruleClient.AddRule(ctx, &pb.AddRuleRequest{
		Username: req.Username,
		Rule: &pb.Rule{
			Field:       req.Field,
			Operator:    req.Operator,
			Value:       req.Value,
			Action:      req.Action,
			ActionValue: req.ActionValue,
		},
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"success": resp.Success, "message": resp.Message})
}

func (a *api) handleDeleteRule(w http.ResponseWriter, r *http.Request) {
	username := r.URL.Query().Get("username")
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil || username == "" {
		writeErr(w, http.StatusBadRequest, errAny("valid rule id and username are required"))
		return
	}
	if err := a.rStore.DeleteRule(username, id); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// --- Email log ---

func (a *api) handleRecentEmails(w http.ResponseWriter, r *http.Request) {
	username := r.URL.Query().Get("username")
	if username == "" {
		writeErr(w, http.StatusBadRequest, errAny("username query param is required"))
		return
	}
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil {
			limit = parsed
		}
	}
	emails, err := a.rStore.GetRecentEmails(username, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, emails)
}

func (a *api) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleReadEmail returns the full stored content (headers + body) of a
// single received email, after checking that it actually belongs to the
// requesting user (mongo_doc_id/email_meta.id are the same value).
func (a *api) handleReadEmail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	username := r.URL.Query().Get("username")
	if id == "" || username == "" {
		writeErr(w, http.StatusBadRequest, errAny("email id and username are required"))
		return
	}

	recipient, err := a.rStore.GetEmailRecipient(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	// Recipient may be a joined "a@x.com, b@x.com" list for multi-recipient mail.
	if !strings.Contains(recipient, username) {
		writeErr(w, http.StatusForbidden, errAny("you do not have access to this email"))
		return
	}

	payload, err := a.dStore.GetPayload(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, errAny("email content not found"))
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// --- Compose / send ---

type sendMailRequest struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

func (a *api) handleSendMail(w http.ResponseWriter, r *http.Request) {
	var req sendMailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if req.From == "" || req.To == "" {
		writeErr(w, http.StatusBadRequest, errAny("from and to are required"))
		return
	}

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s\r\n",
		req.From, req.To, req.Subject, req.Body)

	// The server doesn't require AUTH for local delivery (see smtp.Backend),
	// so no auth is passed here — this mirrors a plain unauthenticated relay.
	if err := smtp.SendMail(a.smtpAddr, nil, req.From, []string{req.To}, []byte(msg)); err != nil {
		writeErr(w, http.StatusBadGateway, fmt.Errorf("delivery failed: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}

type simpleErr string

func (e simpleErr) Error() string { return string(e) }
func errAny(msg string) error     { return simpleErr(msg) }

func main() {
	pgConn := getenv("POSTGRES_DSN", "postgres://smtp_user:smtp_password@127.0.0.1:5432/smtp_metadata?sslmode=disable")
	mongoURI := getenv("MONGO_URI", "mongodb://127.0.0.1:27017")
	controlAddr := getenv("CONTROL_GRPC_ADDR", "127.0.0.1:9090")
	smtpAddr := getenv("SMTP_ADDR", "127.0.0.1:1025")
	httpAddr := getenv("API_ADDR", "0.0.0.0:8080")
	corsOrigin := getenv("CORS_ALLOW_ORIGIN", "*")

	rStore, err := db.NewPostgresService(pgConn)
	if err != nil {
		log.Fatalf("CRITICAL: Postgres connection failure: %v", err)
	}
	defer rStore.Close()

	dStore, err := db.NewMongoService(mongoURI, "mail_vault", "payloads")
	if err != nil {
		log.Fatalf("CRITICAL: MongoDB connection failure: %v", err)
	}
	defer dStore.Close()

	conn, err := grpc.NewClient(controlAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("CRITICAL: failed to dial controld at %s: %v", controlAddr, err)
	}
	defer conn.Close()

	a := &api{rStore: rStore, dStore: dStore, ruleClient: pb.NewRuleServiceClient(conn), smtpAddr: smtpAddr}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"service": "gomail apiserver",
			"note":    "this is the JSON API — the web UI is served separately (see the web/ service, default http://localhost:5173)",
			"health":  "/api/health",
		})
	})
	mux.HandleFunc("GET /api/health", a.handleHealth)
	mux.HandleFunc("POST /api/signup", a.handleSignup)
	mux.HandleFunc("POST /api/login", a.handleLogin)
	mux.HandleFunc("GET /api/rules", a.handleListRules)
	mux.HandleFunc("POST /api/rules", a.handleAddRule)
	mux.HandleFunc("DELETE /api/rules/{id}", a.handleDeleteRule)
	mux.HandleFunc("GET /api/emails", a.handleRecentEmails)
	mux.HandleFunc("GET /api/emails/{id}", a.handleReadEmail)
	mux.HandleFunc("POST /api/send", a.handleSendMail)

	log.Printf("[apiserver] REST gateway listening on %s (control-plane: %s, smtp: %s)", httpAddr, controlAddr, smtpAddr)
	if err := http.ListenAndServe(httpAddr, withCORS(corsOrigin, mux)); err != nil {
		log.Fatalf("apiserver crash: %v", err)
	}
}
