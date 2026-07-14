package smtp

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"log"
	"mime"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/emersion/go-sasl"
	gosmtp "github.com/emersion/go-smtp"
	"gomail.com/db"
)

type Backend struct {
	RStore db.RelationalStore
	DStore db.DocumentStore
}

func (bkd *Backend) NewSession(c *gosmtp.Conn) (gosmtp.Session, error) {
	return &Session{backend: bkd}, nil
}

type Session struct {
	backend   *Backend
	From      string
	To        []string
	username  string
}

// AuthMechanisms implements gosmtp.AuthSession — advertises PLAIN in EHLO.
func (s *Session) AuthMechanisms() []string { return []string{sasl.Plain} }

// Auth implements gosmtp.AuthSession — validates credentials via Postgres.
func (s *Session) Auth(mech string) (sasl.Server, error) {
	if mech != sasl.Plain {
		return nil, errors.New("unsupported auth mechanism")
	}
	return sasl.NewPlainServer(func(identity, username, password string) error {
		if err := s.backend.RStore.Authenticate(username, password); err != nil {
			return errors.New("authentication failed")
		}
		s.username = username
		return nil
	}), nil
}

func (s *Session) Mail(from string, opts *gosmtp.MailOptions) error { s.From = from; return nil }
func (s *Session) Rcpt(to string, opts *gosmtp.RcptOptions) error   { s.To = append(s.To, to); return nil }

func (s *Session) Data(r io.Reader) error {
	if s.backend.DStore == nil || s.backend.RStore == nil {
		return errors.New("internal storage engine offline")
	}

	raw, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	rawStr := string(raw)

	headers, subject, body := parseEmail(rawStr)
	preview := buildPreview(body, 160)

	primaryRecipient := ""
	if len(s.To) > 0 {
		primaryRecipient = s.To[0]
	}

	rules, err := s.backend.RStore.GetRulesForUser(primaryRecipient)
	if err != nil {
		log.Printf("[FILTER-WARNING] Could not fetch rules for %s: %v", primaryRecipient, err)
	}

	targetFolder, isSpam := ProcessRules(rules, s.From, rawStr)
	log.Printf("[FILTER-ENGINE] Routing email to Folder: %s (Spam: %v)", targetFolder, isSpam)

	uuidBytes := make([]byte, 16)
	_, _ = rand.Read(uuidBytes)
	msgID := hex.EncodeToString(uuidBytes)

	if err := s.backend.DStore.SavePayload(msgID, body, headers); err != nil {
		return err
	}

	meta := &db.EmailMeta{
		ID:         msgID,
		Sender:     s.From,
		Recipient:  strings.Join(s.To, ", "),
		MongoDocID: msgID,
		Folder:     targetFolder,
		IsSpam:     isSpam,
		ReceivedAt: time.Now(),
		Subject:    subject,
		Preview:    preview,
	}
	return s.backend.RStore.SaveMetadata(meta)
}

func (s *Session) Reset()        { s.From = ""; s.To = nil }
func (s *Session) Logout() error { return nil }

func parseEmail(raw string) (headers map[string]string, subject, body string) {
	headers = map[string]string{"X-Server": "GoSMTP"}

	msg, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		return headers, "", raw
	}

	dec := new(mime.WordDecoder)
	for key, vals := range msg.Header {
		joined := strings.Join(vals, ", ")
		if decoded, err := dec.DecodeHeader(joined); err == nil {
			joined = decoded
		}
		headers[key] = joined
	}
	headers["X-Server"] = "GoSMTP"

	subject = headers["Subject"]

	bodyBytes, err := io.ReadAll(msg.Body)
	if err == nil {
		body = string(bodyBytes)
	} else {
		body = raw
	}
	return
}

func stripHTML(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
			b.WriteRune(' ')
		case !inTag:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func buildPreview(body string, maxRunes int) string {
	s := strings.Join(strings.Fields(stripHTML(body)), " ")
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes]) + "…"
}
