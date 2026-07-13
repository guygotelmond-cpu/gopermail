package smtp

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"log"

	"strings"
	"time"

	"github.com/emersion/go-smtp"
	"gomail.com/db"
)

type Backend struct {
	RStore db.RelationalStore // Injected abstraction
	DStore db.DocumentStore   // Injected abstraction
}

func (bkd *Backend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	return &Session{backend: bkd}, nil
}

type Session struct {
	backend *Backend
	From    string
	To      []string
}

func (s *Session) AuthMechanisms() []string { return []string{"PLAIN"} }

func (s *Session) Auth(mech string, username, password string) (smtp.Session, error) {
	if err := s.backend.RStore.Authenticate(username, password); err != nil {
		return nil, errors.New("authentication failed")
	}
	return s, nil
}

func (s *Session) Mail(from string, opts *smtp.MailOptions) error { s.From = from; return nil }
func (s *Session) Rcpt(to string, opts *smtp.RcptOptions) error   { s.To = append(s.To, to); return nil }

func (s *Session) Data(r io.Reader) error {
	if s.backend.DStore == nil || s.backend.RStore == nil {
		return errors.New("internal storage engine offline")
	}

	buf := new(strings.Builder)
	if _, err := io.Copy(buf, r); err != nil {
		return err
	}
	rawEmailContent := buf.String()

	// 1. Fetch dynamic inbox filters from Postgres for the primary recipient
	// (Using the first recipient in the slice as the primary mailbox owner)
	primaryRecipient := ""
	if len(s.To) > 0 {
		primaryRecipient = s.To[0]
	}

	rules, err := s.backend.RStore.GetRulesForUser(primaryRecipient)
	if err != nil {
		log.Printf("[FILTER-WARNING] Could not fetch rules for %s: %v", primaryRecipient, err)
	}

	// 2. Run the pipeline through the Filtering engine
	targetFolder, isSpam := ProcessRules(rules, s.From, rawEmailContent)
	log.Printf("[FILTER-ENGINE] Routing email to Folder: %s (Spam: %v)", targetFolder, isSpam)

	// 3. Generate tracking IDs
	uuidBytes := make([]byte, 16)
	_, _ = rand.Read(uuidBytes)
	msgID := hex.EncodeToString(uuidBytes)

	// 4. Save heavy body to MongoDB
	err = s.backend.DStore.SavePayload(msgID, rawEmailContent, map[string]string{"X-Server": "GoSMTP"})
	if err != nil {
		return err
	}

	// 5. Update Postgres EmailMeta to support new structural folder mapping
	meta := &db.EmailMeta{
		ID:         msgID,
		Sender:     s.From,
		Recipient:  strings.Join(s.To, ", "),
		MongoDocID: msgID,
		Folder:     targetFolder, // Saved dynamically!
		IsSpam:     isSpam,       // Saved dynamically!
		ReceivedAt: time.Now(),
	}

	// Make sure to update your Postgres SaveMetadata function parameters to handle folder and isSpam
	return s.backend.RStore.SaveMetadata(meta)
}

func (s *Session) Reset()        { s.From = ""; s.To = nil }
func (s *Session) Logout() error { return nil }
