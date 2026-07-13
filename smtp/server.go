package smtp

import (
	"crypto/tls"
	"errors"
	"io"
	"log"
	"time"

	"github.com/emersion/go-smtp"
)

// Config holds dynamic settings for running the server in different environments.
type Config struct {
	Addr            string
	Domain          string
	MaxMessageBytes int64
	MaxRecipients   int
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	EnableTLS       bool
	CertFile        string
	KeyFile         string
	AllowInsecure   bool // ONLY true in local development/testing environments
}

// NewServer configures and returns a secure, robust operational SMTP server instance.
func NewServer(backend *Backend, cfg Config) (*smtp.Server, error) {
	if backend == nil {
		return nil, errors.New("cannot initialize SMTP server with a nil backend")
	}

	s := smtp.NewServer(backend)

	// --- Dynamic Settings Mapping ---
	s.Addr = cfg.Addr
	s.Domain = cfg.Domain
	s.MaxMessageBytes = cfg.MaxMessageBytes
	s.MaxRecipients = cfg.MaxRecipients
	s.ReadTimeout = cfg.ReadTimeout
	s.WriteTimeout = cfg.WriteTimeout
	s.AllowInsecureAuth = cfg.AllowInsecure
	s.ErrorLog = log.New(io.Discard, "", 0)
	// --- Security hardening ---
	if cfg.EnableTLS {
		// Load system certificate/key files for TLS handshake encryption
		cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return nil, errors.New("failed to load TLS certificates: " + err.Error())
		}

		s.TLSConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12, // Block old, vulnerable TLS versions (1.0 & 1.1)
		}

		if !cfg.AllowInsecure {
			s.LMTP = false
		}
		log.Println("[SMTP-INIT] TLS Layer successfully loaded into SMTP lifecycle.")
	} else if !cfg.AllowInsecure {
		// Stop execution if trying to deploy to production without encryption
		return nil, errors.New("security risk: cannot run production SMTP without enabling TLS configurations")
	}

	return s, nil
}
