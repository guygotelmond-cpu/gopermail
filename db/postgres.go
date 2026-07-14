package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

// EmailMeta matches the normalized architectural tracking needs.

type Rule struct {
	ID          int
	Field       string
	Operator    string
	Value       string
	Action      string
	ActionValue string
}

type PostgresService struct {
	db *sql.DB
}

// runMigration serializes DDL across concurrently-starting services using a
// Postgres session-level advisory lock (magic key 0x676d61696c = "gmail").
// Without this, CREATE TABLE IF NOT EXISTS can race and hit a duplicate-type
// constraint error in the pg_type system catalog.
func runMigration(db *sql.DB, schema string) error {
	conn, err := db.Conn(context.Background())
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(context.Background(), "SELECT pg_advisory_lock(7070069868)"); err != nil {
		return err
	}
	defer conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock(7070069868)") //nolint:errcheck
	_, err = conn.ExecContext(context.Background(), schema)
	return err
}

func NewPostgresService(connStr string) (*PostgresService, error) {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres connection: %w", err)
	}

	const schema = `
	CREATE TABLE IF NOT EXISTS users (
		id SERIAL PRIMARY KEY,
		username TEXT UNIQUE NOT NULL,
		password TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS email_meta (
		id TEXT PRIMARY KEY,
		sender TEXT NOT NULL,
		recipient TEXT NOT NULL,
		mongo_doc_id TEXT NOT NULL,
		folder TEXT DEFAULT 'INBOX',
		is_spam BOOLEAN DEFAULT FALSE,
		received_at TIMESTAMP NOT NULL,
		subject TEXT DEFAULT '',
		preview TEXT DEFAULT ''
	);

	CREATE TABLE IF NOT EXISTS rules (
		id SERIAL PRIMARY KEY,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		field TEXT NOT NULL CONSTRAINT chk_rule_field CHECK (field IN ('sender', 'subject', 'body')),
		operator TEXT NOT NULL CONSTRAINT chk_rule_operator CHECK (operator IN ('contains', 'equals')),
		value TEXT NOT NULL,
		action TEXT NOT NULL CONSTRAINT chk_rule_action CHECK (action IN ('move_to', 'mark_spam', 'delete')),
		action_value TEXT
	);

	ALTER TABLE email_meta ADD COLUMN IF NOT EXISTS subject TEXT DEFAULT '';
	ALTER TABLE email_meta ADD COLUMN IF NOT EXISTS preview TEXT DEFAULT '';
	ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_secret TEXT NOT NULL DEFAULT '';
	ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_enabled BOOLEAN NOT NULL DEFAULT FALSE;
	`

	if err := runMigration(db, schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to execute normalized database migration schema: %w", err)
	}

	return &PostgresService{db: db}, nil
}

func (p *PostgresService) Authenticate(username, password string) error {
	var hash string
	err := p.db.QueryRow("SELECT password FROM users WHERE username = $1", username).Scan(&hash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("invalid credentials")
		}
		return fmt.Errorf("authentication lookup error: %w", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return errors.New("invalid credentials")
	}
	return nil
}

// CreateUser registers a new mailbox owner. Used by the API service for signup.
func (p *PostgresService) CreateUser(username, password string) error {
	if username == "" || password == "" {
		return errors.New("username and password are required")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}
	_, err = p.db.Exec("INSERT INTO users (username, password) VALUES ($1, $2)", username, string(hash))
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}
	return nil
}

func (p *PostgresService) SaveMetadata(meta *EmailMeta) error {
	query := `
		INSERT INTO email_meta (id, sender, recipient, mongo_doc_id, folder, is_spam, received_at, subject, preview)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	_, err := p.db.Exec(
		query,
		meta.ID, meta.Sender, meta.Recipient, meta.MongoDocID,
		meta.Folder, meta.IsSpam, meta.ReceivedAt, meta.Subject, meta.Preview,
	)
	if err != nil {
		return fmt.Errorf("failed to save email transaction metadata: %w", err)
	}
	// Notify SSE listeners with the full stored recipient value.
	// The hub matches by comparing local parts, so guygo@gmail.com == guygo.
	_, _ = p.db.Exec("SELECT pg_notify('new_mail', $1)", meta.Recipient)
	return nil
}

// GetRecentEmails returns the most recent inbound emails for a recipient, used by the frontend mail log view.
func (p *PostgresService) GetRecentEmails(username string, limit int) ([]EmailMeta, error) {
	if limit <= 0 {
		limit = 50
	}
	// Match bare username OR username@anydomain — handles internal compose
	// (stores 'alice') and external SMTP delivery (stores 'alice@localhost').
	query := `
		SELECT id, sender, recipient, mongo_doc_id, folder, is_spam, received_at, subject, preview
		FROM email_meta
		WHERE recipient = $1 OR recipient LIKE $1 || '@%'
		ORDER BY received_at DESC
		LIMIT $2
	`
	rows, err := p.db.Query(query, username, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query recent emails: %w", err)
	}
	defer rows.Close()

	var out []EmailMeta
	for rows.Next() {
		var m EmailMeta
		if err := rows.Scan(
			&m.ID, &m.Sender, &m.Recipient, &m.MongoDocID,
			&m.Folder, &m.IsSpam, &m.ReceivedAt, &m.Subject, &m.Preview,
		); err != nil {
			return nil, fmt.Errorf("failed scanning email_meta row: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failure: %w", err)
	}
	return out, nil
}

func (p *PostgresService) UpdatePassword(username, newPassword string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}
	res, err := p.db.Exec("UPDATE users SET password = $1 WHERE username = $2", string(hash), username)
	if err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("user not found")
	}
	return nil
}

// GetEmailRecipient returns the recipient string stored for a given email_meta id,
// used to verify a user is only allowed to read their own mail.
func (p *PostgresService) GetEmailRecipient(id string) (string, error) {
	var recipient string
	err := p.db.QueryRow("SELECT recipient FROM email_meta WHERE id = $1", id).Scan(&recipient)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", errors.New("email not found")
		}
		return "", fmt.Errorf("failed looking up email owner: %w", err)
	}
	return recipient, nil
}

func (p *PostgresService) GetRulesForUser(username string) ([]Rule, error) {
	query := `
		SELECT r.id, r.field, r.operator, r.value, r.action, r.action_value 
		FROM rules r
		JOIN users u ON r.user_id = u.id
		WHERE u.username = $1
		ORDER BY r.id ASC
	`
	rows, err := p.db.Query(query, username)
	if err != nil {
		return nil, fmt.Errorf("failed to execute rules lookup: %w", err)
	}
	defer rows.Close()

	var rules []Rule
	for rows.Next() {
		var r Rule
		if err := rows.Scan(&r.ID, &r.Field, &r.Operator, &r.Value, &r.Action, &r.ActionValue); err != nil {
			return nil, fmt.Errorf("failed scanning rule entity row: %w", err)
		}
		rules = append(rules, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failure: %w", err)
	}
	return rules, nil
}

func (p *PostgresService) AddRule(username string, rule Rule) error {
	if err := rule.Validate(); err != nil {
		return fmt.Errorf("rule validation failed: %w", err)
	}

	// 1. Resolve normalized user_id first
	var userID int
	err := p.db.QueryRow("SELECT id FROM users WHERE username = $1", username).Scan(&userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("cannot add rule: target user profile does not exist")
		}
		return fmt.Errorf("user resolution fault: %w", err)
	}

	// 2. Insert with relation key reference mapping
	query := `
		INSERT INTO rules (user_id, field, operator, value, action, action_value) 
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	_, err = p.db.Exec(query, userID, rule.Field, rule.Operator, rule.Value, rule.Action, rule.ActionValue)
	if err != nil {
		return fmt.Errorf("failed writing rule definition to storage: %w", err)
	}
	return nil
}

// DeleteRule removes a rule owned by username. Scoped by username so one user can't delete another's rule.
func (p *PostgresService) DeleteRule(username string, ruleID int) error {
	query := `
		DELETE FROM rules
		WHERE id = $1 AND user_id = (SELECT id FROM users WHERE username = $2)
	`
	res, err := p.db.Exec(query, ruleID, username)
	if err != nil {
		return fmt.Errorf("failed deleting rule: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed checking delete result: %w", err)
	}
	if affected == 0 {
		return errors.New("rule not found for this user")
	}
	return nil
}

func (p *PostgresService) GetTOTPStatus(username string) (string, bool, error) {
	var secret string
	var enabled bool
	err := p.db.QueryRow(
		"SELECT totp_secret, totp_enabled FROM users WHERE username = $1", username,
	).Scan(&secret, &enabled)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, errors.New("user not found")
		}
		return "", false, fmt.Errorf("totp status lookup failed: %w", err)
	}
	return secret, enabled, nil
}

func (p *PostgresService) SetTOTPSecret(username, secret string) error {
	res, err := p.db.Exec(
		"UPDATE users SET totp_secret = $1, totp_enabled = FALSE WHERE username = $2",
		secret, username,
	)
	if err != nil {
		return fmt.Errorf("failed to set totp secret: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("user not found")
	}
	return nil
}

func (p *PostgresService) EnableTOTP(username string) error {
	res, err := p.db.Exec(
		"UPDATE users SET totp_enabled = TRUE WHERE username = $1 AND totp_secret != ''",
		username,
	)
	if err != nil {
		return fmt.Errorf("failed to enable totp: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("no TOTP secret configured for user")
	}
	return nil
}

func (p *PostgresService) DisableTOTP(username string) error {
	res, err := p.db.Exec(
		"UPDATE users SET totp_secret = '', totp_enabled = FALSE WHERE username = $1",
		username,
	)
	if err != nil {
		return fmt.Errorf("failed to disable totp: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("user not found")
	}
	return nil
}

func (p *PostgresService) Close() error {
	return p.db.Close()
}

func (r *Rule) Validate() error {
	switch r.Field {
	case "sender", "subject", "body":
	default:
		return errors.New("invalid field: must be 'sender', 'subject', or 'body'")
	}

	switch r.Operator {
	case "contains", "equals":
	default:
		return errors.New("invalid operator: must be 'contains' or 'equals'")
	}

	switch r.Action {
	case "move_to", "mark_spam", "delete":
		if r.Action == "move_to" && r.ActionValue == "" {
			return errors.New("action 'move_to' requires a destination folder name")
		}
	default:
		return errors.New("invalid action: must be 'move_to', 'mark_spam', or 'delete'")
	}

	return nil
}
