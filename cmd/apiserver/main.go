// Command apiserver is the browser-facing REST gateway. Reads (rules, email
// log, auth) go straight to Postgres; writes to rules are proxied through
// controld's gRPC AddRule so rule mutation stays centralized in one place.
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image/png"
	"log"
	"net/http"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/lib/pq"
	"github.com/pquerna/otp/totp"
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

// ─── SSE hub ─────────────────────────────────────────────────────────────────

type subscriber struct {
	username string
	ch       chan struct{}
}

// hub tracks active SSE connections. Matching is done by local part of the
// address so "guygo@gmail.com" (stored recipient) hits a client subscribed
// as "guygo@gmail.com", and bare "guygo" (compose-to) hits the same client.
type hub struct {
	mu      sync.RWMutex
	clients []*subscriber
}

func newHub() *hub { return &hub{} }

func localPart(addr string) string {
	if i := strings.Index(addr, "@"); i >= 0 {
		return strings.ToLower(addr[:i])
	}
	return strings.ToLower(addr)
}

func (h *hub) subscribe(username string) (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	s := &subscriber{username: username, ch: ch}
	h.mu.Lock()
	h.clients = append(h.clients, s)
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		for i, c := range h.clients {
			if c == s {
				h.clients = append(h.clients[:i], h.clients[i+1:]...)
				return
			}
		}
	}
}

// notify fans out to every subscriber whose local part matches the recipient's
// local part. Snapshot taken under read-lock so writes don't stall readers.
func (h *hub) notify(recipient string) {
	rLocal := localPart(recipient)
	h.mu.RLock()
	snapshot := make([]*subscriber, len(h.clients))
	copy(snapshot, h.clients)
	h.mu.RUnlock()
	for _, s := range snapshot {
		if localPart(s.username) == rLocal {
			select {
			case s.ch <- struct{}{}:
			default:
			}
		}
	}
}

// startMailListener opens a dedicated Postgres connection and blocks on
// LISTEN new_mail, routing each notification to the hub.
func startMailListener(pgConn string, h *hub) {
	report := func(ev pq.ListenerEventType, err error) {
		if err != nil {
			log.Printf("[sse-listener] pg error: %v", err)
		}
	}
	l := pq.NewListener(pgConn, 10*time.Second, time.Minute, report)
	if err := l.Listen("new_mail"); err != nil {
		log.Printf("[sse-listener] LISTEN failed: %v", err)
		return
	}
	log.Printf("[sse-listener] ready — waiting for new_mail notifications")
	for n := range l.Notify {
		if n == nil {
			continue // keepalive ping from pq
		}
		h.notify(n.Extra)
	}
}

// ─── Rate limiter ─────────────────────────────────────────────────────────────

type rlEntry struct {
	count     int
	windowEnd time.Time
}

type rateLimiter struct {
	mu      sync.Mutex
	entries map[string]*rlEntry
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{entries: make(map[string]*rlEntry)}
}

func (rl *rateLimiter) allow(ip string, max int, window time.Duration) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	e, ok := rl.entries[ip]
	if !ok || now.After(e.windowEnd) {
		rl.entries[ip] = &rlEntry{count: 1, windowEnd: now.Add(window)}
		return true
	}
	if e.count >= max {
		return false
	}
	e.count++
	return true
}

// wrap returns a handler that rate-limits by remote IP before calling fn.
func (rl *rateLimiter) wrap(max int, window time.Duration, fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		// Strip port if present.
		if i := strings.LastIndex(ip, ":"); i > 0 {
			ip = ip[:i]
		}
		if !rl.allow(ip, max, window) {
			writeErr(w, http.StatusTooManyRequests, errAny("too many requests, try again later"))
			return
		}
		fn(w, r)
	}
}

// ─── API server ───────────────────────────────────────────────────────────────

type ctxKey string

const ctxUsername ctxKey = "username"

type api struct {
	rStore     db.RelationalStore
	dStore     db.DocumentStore
	ruleClient pb.RuleServiceClient
	smtpAddr   string
	hub        *hub
	jwtSecret  []byte
	rl         *rateLimiter
}

// mailClaims is the JWT payload for both full sessions and pending-TOTP tokens.
type mailClaims struct {
	jwt.RegisteredClaims
	TOTPPending bool `json:"totp_pending,omitempty"`
}

// tokenForUser issues a full session JWT for the given username, valid 24h.
func (a *api) tokenForUser(username string) (string, error) {
	c := mailClaims{RegisteredClaims: jwt.RegisteredClaims{
		Subject:   username,
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(a.jwtSecret)
}

// partialTokenForUser issues a short-lived token that marks TOTP verification
// as pending. It cannot access protected endpoints — only /api/totp/login.
func (a *api) partialTokenForUser(username string) (string, error) {
	c := mailClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   username,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		TOTPPending: true,
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(a.jwtSecret)
}

// parseClaims validates a JWT and returns its claims.
func (a *api) parseClaims(tokenStr string) (*mailClaims, error) {
	c := &mailClaims{}
	tok, err := jwt.ParseWithClaims(tokenStr, c, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errAny("unexpected signing method")
		}
		return a.jwtSecret, nil
	})
	if err != nil || !tok.Valid {
		return nil, errAny("invalid or expired token")
	}
	return c, nil
}

// auth wraps a handler, requiring a valid full (non-pending) Bearer token.
func (a *api) auth(fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hdr := r.Header.Get("Authorization")
		if !strings.HasPrefix(hdr, "Bearer ") {
			writeErr(w, http.StatusUnauthorized, errAny("missing or invalid authorization header"))
			return
		}
		c, err := a.parseClaims(hdr[7:])
		if err != nil {
			writeErr(w, http.StatusUnauthorized, err)
			return
		}
		if c.TOTPPending {
			writeErr(w, http.StatusUnauthorized, errAny("TOTP verification required"))
			return
		}
		ctx := context.WithValue(r.Context(), ctxUsername, c.Subject)
		fn(w, r.WithContext(ctx))
	}
}

// authFromQuery is like auth but accepts the token as a ?token= query param.
// Used only for the SSE endpoint since EventSource cannot set headers.
func (a *api) authFromQuery(fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokenStr := r.URL.Query().Get("token")
		if tokenStr == "" {
			writeErr(w, http.StatusUnauthorized, errAny("missing token query param"))
			return
		}
		c, err := a.parseClaims(tokenStr)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, err)
			return
		}
		if c.TOTPPending {
			writeErr(w, http.StatusUnauthorized, errAny("TOTP verification required"))
			return
		}
		ctx := context.WithValue(r.Context(), ctxUsername, c.Subject)
		fn(w, r.WithContext(ctx))
	}
}

func usernameFromCtx(ctx context.Context) string {
	v, _ := ctx.Value(ctxUsername).(string)
	return v
}

// ─── HTTP helpers ─────────────────────────────────────────────────────────────

func withCORS(allowOrigin string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", allowOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func withBodyLimit(maxBytes int64, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
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

// ─── Auth handlers ────────────────────────────────────────────────────────────

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
	_, totpEnabled, err := a.rStore.GetTOTPStatus(req.Username)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if totpEnabled {
		partial, err := a.partialTokenForUser(req.Username)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, errAny("failed to issue token"))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"requires_totp": true,
			"partial_token": partial,
		})
		return
	}
	token, err := a.tokenForUser(req.Username)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, errAny("failed to issue token"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "token": token, "username": req.Username})
}

// ─── TOTP handlers ────────────────────────────────────────────────────────────

// handleTOTPStatus returns whether TOTP is currently enabled for the user.
func (a *api) handleTOTPStatus(w http.ResponseWriter, r *http.Request) {
	username := usernameFromCtx(r.Context())
	_, enabled, err := a.rStore.GetTOTPStatus(username)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"enabled": enabled})
}

// handleTOTPSetup generates a new TOTP secret, stores it (not yet enabled),
// and returns a QR code image as a base64 PNG data URL.
func (a *api) handleTOTPSetup(w http.ResponseWriter, r *http.Request) {
	username := usernameFromCtx(r.Context())
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "GoPerMail",
		AccountName: username,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, fmt.Errorf("failed to generate TOTP key: %w", err))
		return
	}
	if err := a.rStore.SetTOTPSecret(username, key.Secret()); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	img, err := key.Image(200, 200)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, fmt.Errorf("failed to render QR code: %w", err))
		return
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		writeErr(w, http.StatusInternalServerError, fmt.Errorf("failed to encode QR code: %w", err))
		return
	}
	qr := "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
	writeJSON(w, http.StatusOK, map[string]string{"qr": qr, "secret": key.Secret()})
}

// handleTOTPEnable confirms the TOTP setup by validating a code, then enables it.
func (a *api) handleTOTPEnable(w http.ResponseWriter, r *http.Request) {
	username := usernameFromCtx(r.Context())
	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	secret, _, err := a.rStore.GetTOTPStatus(username)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if secret == "" {
		writeErr(w, http.StatusBadRequest, errAny("no TOTP secret set up, call /api/totp/setup first"))
		return
	}
	if !totp.Validate(req.Code, secret) {
		writeErr(w, http.StatusUnauthorized, errAny("invalid TOTP code"))
		return
	}
	if err := a.rStore.EnableTOTP(username); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "enabled"})
}

// handleTOTPDisable disables TOTP after verifying password + current TOTP code.
func (a *api) handleTOTPDisable(w http.ResponseWriter, r *http.Request) {
	username := usernameFromCtx(r.Context())
	var req struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := a.rStore.Authenticate(username, req.Password); err != nil {
		writeErr(w, http.StatusUnauthorized, errAny("incorrect password"))
		return
	}
	secret, enabled, err := a.rStore.GetTOTPStatus(username)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if !enabled {
		writeErr(w, http.StatusBadRequest, errAny("TOTP is not enabled"))
		return
	}
	if !totp.Validate(req.Code, secret) {
		writeErr(w, http.StatusUnauthorized, errAny("invalid TOTP code"))
		return
	}
	if err := a.rStore.DisableTOTP(username); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "disabled"})
}

// handleTOTPLogin completes a login that requires TOTP. It accepts a partial
// token (issued by /api/login when TOTP is enabled) plus the 6-digit code,
// validates them, and issues a full session token.
func (a *api) handleTOTPLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PartialToken string `json:"partial_token"`
		Code         string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	c, err := a.parseClaims(req.PartialToken)
	if err != nil || !c.TOTPPending {
		writeErr(w, http.StatusUnauthorized, errAny("invalid or expired partial token"))
		return
	}
	username := c.Subject
	secret, enabled, err := a.rStore.GetTOTPStatus(username)
	if err != nil || !enabled {
		writeErr(w, http.StatusUnauthorized, errAny("TOTP not available for this account"))
		return
	}
	if !totp.Validate(req.Code, secret) {
		writeErr(w, http.StatusUnauthorized, errAny("invalid TOTP code"))
		return
	}
	token, err := a.tokenForUser(username)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, errAny("failed to issue token"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "token": token, "username": username})
}

// ─── Rules handlers ───────────────────────────────────────────────────────────

type ruleDTO struct {
	ID          int    `json:"id"`
	Field       string `json:"field"`
	Operator    string `json:"operator"`
	Value       string `json:"value"`
	Action      string `json:"action"`
	ActionValue string `json:"action_value"`
}

func (a *api) handleListRules(w http.ResponseWriter, r *http.Request) {
	username := usernameFromCtx(r.Context())
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
	Field       string `json:"field"`
	Operator    string `json:"operator"`
	Value       string `json:"value"`
	Action      string `json:"action"`
	ActionValue string `json:"action_value"`
}

func (a *api) handleAddRule(w http.ResponseWriter, r *http.Request) {
	username := usernameFromCtx(r.Context())
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
		Username: username,
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
	username := usernameFromCtx(r.Context())
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeErr(w, http.StatusBadRequest, errAny("valid rule id is required"))
		return
	}
	if err := a.rStore.DeleteRule(username, id); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ─── Email handlers ───────────────────────────────────────────────────────────

func (a *api) handleRecentEmails(w http.ResponseWriter, r *http.Request) {
	username := usernameFromCtx(r.Context())
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

// handleEvents is the SSE endpoint. It blocks until the client disconnects,
// writing "data: new_mail\n\n" whenever the hub delivers a notification.
// Auth is done via ?token= query param because EventSource cannot set headers.
func (a *api) handleEvents(w http.ResponseWriter, r *http.Request) {
	username := usernameFromCtx(r.Context())

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, errAny("streaming not supported"))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// Tell nginx not to buffer this response.
	w.Header().Set("X-Accel-Buffering", "no")

	ch, unsub := a.hub.subscribe(username)
	defer unsub()

	// Initial ping so the client knows the connection is alive.
	fmt.Fprintf(w, "data: connected\n\n")
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ch:
			fmt.Fprintf(w, "data: new_mail\n\n")
			flusher.Flush()
		}
	}
}

func (a *api) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleReadEmail returns the full stored content of a single received email,
// after verifying it belongs to the authenticated user.
func (a *api) handleReadEmail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	username := usernameFromCtx(r.Context())
	if id == "" {
		writeErr(w, http.StatusBadRequest, errAny("email id is required"))
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

// ─── Send mail ────────────────────────────────────────────────────────────────

type sendMailRequest struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
	IsHTML  bool   `json:"is_html"`
}

func (a *api) handleSendMail(w http.ResponseWriter, r *http.Request) {
	authUser := usernameFromCtx(r.Context())
	var req sendMailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if req.From == "" || req.To == "" {
		writeErr(w, http.StatusBadRequest, errAny("from and to are required"))
		return
	}
	// Verify the From address belongs to the authenticated user.
	if localPart(req.From) != localPart(authUser) {
		writeErr(w, http.StatusForbidden, errAny("from address does not match authenticated user"))
		return
	}

	var msg []byte
	if req.IsHTML {
		msg = []byte(fmt.Sprintf(
			"From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=utf-8\r\n\r\n%s\r\n",
			req.From, req.To, req.Subject, req.Body,
		))
	} else {
		msg = []byte(fmt.Sprintf(
			"From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s\r\n",
			req.From, req.To, req.Subject, req.Body,
		))
	}

	// The server doesn't require AUTH for local delivery (see smtp.Backend),
	// so no auth is passed here — this mirrors a plain unauthenticated relay.
	if err := smtp.SendMail(a.smtpAddr, nil, req.From, []string{req.To}, msg); err != nil {
		writeErr(w, http.StatusBadGateway, fmt.Errorf("delivery failed: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}

// ─── Password change ──────────────────────────────────────────────────────────

type changePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

func (a *api) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	username := usernameFromCtx(r.Context())
	var req changePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if req.OldPassword == "" || req.NewPassword == "" {
		writeErr(w, http.StatusBadRequest, errAny("old_password and new_password are required"))
		return
	}
	if len(req.NewPassword) < 6 {
		writeErr(w, http.StatusBadRequest, errAny("new password must be at least 6 characters"))
		return
	}
	if err := a.rStore.Authenticate(username, req.OldPassword); err != nil {
		writeErr(w, http.StatusUnauthorized, errAny("current password is incorrect"))
		return
	}
	if err := a.rStore.UpdatePassword(username, req.NewPassword); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

type simpleErr string

func (e simpleErr) Error() string { return string(e) }
func errAny(msg string) error     { return simpleErr(msg) }

// ─── Main ─────────────────────────────────────────────────────────────────────

func main() {
	pgConn := getenv("POSTGRES_DSN", "postgres://smtp_user:smtp_password@127.0.0.1:5432/smtp_metadata?sslmode=disable")
	mongoURI := getenv("MONGO_URI", "mongodb://127.0.0.1:27017")
	controlAddr := getenv("CONTROL_GRPC_ADDR", "127.0.0.1:9090")
	smtpAddr := getenv("SMTP_ADDR", "127.0.0.1:1025")
	httpAddr := getenv("API_ADDR", "0.0.0.0:8080")
	corsOrigin := getenv("CORS_ALLOW_ORIGIN", "*")
	jwtSecret := getenv("JWT_SECRET", "")

	if jwtSecret == "" {
		log.Println("WARNING: JWT_SECRET not set — using insecure default. Set JWT_SECRET in production.")
		jwtSecret = "gopermail-dev-secret-change-in-production"
	}

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

	h := newHub()
	go startMailListener(pgConn, h)

	a := &api{
		rStore:     rStore,
		dStore:     dStore,
		ruleClient: pb.NewRuleServiceClient(conn),
		smtpAddr:   smtpAddr,
		hub:        h,
		jwtSecret:  []byte(jwtSecret),
		rl:         newRateLimiter(),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", a.handleHealth)

	// Rate-limited, unauthenticated.
	mux.HandleFunc("POST /api/signup", a.rl.wrap(10, time.Minute, a.handleSignup))
	mux.HandleFunc("POST /api/login", a.rl.wrap(10, time.Minute, a.handleLogin))

	// TOTP step-2 login — accepts a partial token, no full auth required.
	mux.HandleFunc("POST /api/totp/login", a.rl.wrap(10, time.Minute, a.handleTOTPLogin))

	// SSE auth via query param because EventSource cannot set headers.
	mux.HandleFunc("GET /api/events", a.authFromQuery(a.handleEvents))

	// All other endpoints require a valid Bearer token.
	mux.HandleFunc("GET /api/rules", a.auth(a.handleListRules))
	mux.HandleFunc("POST /api/rules", a.auth(a.handleAddRule))
	mux.HandleFunc("DELETE /api/rules/{id}", a.auth(a.handleDeleteRule))
	mux.HandleFunc("GET /api/emails", a.auth(a.handleRecentEmails))
	mux.HandleFunc("GET /api/emails/{id}", a.auth(a.handleReadEmail))
	mux.HandleFunc("POST /api/send", a.auth(a.handleSendMail))
	mux.HandleFunc("PUT /api/password", a.auth(a.handleChangePassword))

	// TOTP management — requires full auth.
	mux.HandleFunc("GET /api/totp/status", a.auth(a.handleTOTPStatus))
	mux.HandleFunc("POST /api/totp/setup", a.auth(a.handleTOTPSetup))
	mux.HandleFunc("POST /api/totp/enable", a.auth(a.handleTOTPEnable))
	mux.HandleFunc("POST /api/totp/disable", a.auth(a.handleTOTPDisable))

	srv := &http.Server{
		Addr:         httpAddr,
		Handler:      withCORS(corsOrigin, withBodyLimit(2<<20, mux)), // 2 MB body limit for HTML mail
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("[apiserver] REST gateway listening on %s (control-plane: %s, smtp: %s)", httpAddr, controlAddr, smtpAddr)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("apiserver crash: %v", err)
	}
}
