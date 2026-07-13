package db

import "time"

// EmailMeta represents structured data optimized for relational indexing.
type EmailMeta struct {
	ID         string    `json:"id"`
	Sender     string    `json:"sender"`
	Recipient  string    `json:"recipient"`
	MongoDocID string    `json:"mongo_doc_id"` // Cross-reference link to MongoDB
	ReceivedAt time.Time `json:"received_at"`
	Folder     string    `json:"folder"`  // Dynamically assigned folder based on filtering rules
	IsSpam     bool      `json:"is_spam"` // Dynamically assigned spam status based on filtering rules
}

// RelationalStore handles auth and quick lookup metadata.
type RelationalStore interface {
	Authenticate(username, password string) error
	CreateUser(username, password string) error
	SaveMetadata(meta *EmailMeta) error
	GetRulesForUser(username string) ([]Rule, error)
	GetEmailRecipient(id string) (string, error)
	DeleteRule(username string, ruleID int) error
	GetRecentEmails(username string, limit int) ([]EmailMeta, error)
	Close() error
	AddRule(username string, rule Rule) error
}

// DocumentStore handles the heavy-lifting document payloads.
type DocumentStore interface {
	SavePayload(id string, body string, headers map[string]string) error
	Close() error
}
