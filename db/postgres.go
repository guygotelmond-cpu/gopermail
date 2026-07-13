package db

import (
	"database/sql"
	"errors"
	"fmt"

	_ "github.com/lib/pq"
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

func NewPostgresService(connStr string) (*PostgresService, error) {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres connection: %w", err)
	}

	// --- 3NF Normalized Schema Implementation ---
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
		received_at TIMESTAMP NOT NULL
	);

	CREATE TABLE IF NOT EXISTS rules (
		id SERIAL PRIMARY KEY,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, -- Normalization Key
		field TEXT NOT NULL CONSTRAINT chk_rule_field CHECK (field IN ('sender', 'subject', 'body')),
		operator TEXT NOT NULL CONSTRAINT chk_rule_operator CHECK (operator IN ('contains', 'equals')),
		value TEXT NOT NULL,
		action TEXT NOT NULL CONSTRAINT chk_rule_action CHECK (action IN ('move_to', 'mark_spam', 'delete')),
		action_value TEXT
	);`

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to execute normalized database migration schema: %w", err)
	}

	return &PostgresService{db: db}, nil
}

func (p *PostgresService) Authenticate(username, password string) error {
	var dbPassword string
	err := p.db.QueryRow("SELECT password FROM users WHERE username = $1", username).Scan(&dbPassword)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("user not found")
		}
		return fmt.Errorf("authentication lookup error: %w", err)
	}

	if dbPassword != password {
		return errors.New("invalid credentials")
	}
	return nil
}

// CreateUser registers a new mailbox owner. Used by the API service for signup.
func (p *PostgresService) CreateUser(username, password string) error {
	if username == "" || password == "" {
		return errors.New("username and password are required")
	}
	_, err := p.db.Exec("INSERT INTO users (username, password) VALUES ($1, $2)", username, password)
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}
	return nil
}

func (p *PostgresService) SaveMetadata(meta *EmailMeta) error {
	query := `
		INSERT INTO email_meta (id, sender, recipient, mongo_doc_id, folder, is_spam, received_at) 
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	_, err := p.db.Exec(
		query,
		meta.ID,
		meta.Sender,
		meta.Recipient,
		meta.MongoDocID,
		meta.Folder,
		meta.IsSpam,
		meta.ReceivedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to save email transaction metadata: %w", err)
	}
	return nil
}

// GetRecentEmails returns the most recent inbound emails for a recipient, used by the frontend mail log view.
func (p *PostgresService) GetRecentEmails(username string, limit int) ([]EmailMeta, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `
		SELECT id, sender, recipient, mongo_doc_id, folder, is_spam, received_at
		FROM email_meta
		WHERE recipient = $1
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
		if err := rows.Scan(&m.ID, &m.Sender, &m.Recipient, &m.MongoDocID, &m.Folder, &m.IsSpam, &m.ReceivedAt); err != nil {
			return nil, fmt.Errorf("failed scanning email_meta row: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failure: %w", err)
	}
	return out, nil
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
