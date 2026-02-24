package mailer

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"
)

type Service struct {
	host     string
	port     int
	username string
	password string
	from     string
}

func New(host string, port int, username, password, from string) *Service {
	return &Service{
		host:     strings.TrimSpace(host),
		port:     port,
		username: strings.TrimSpace(username),
		password: password,
		from:     strings.TrimSpace(from),
	}
}

func (s *Service) Enabled() bool {
	return s.host != "" && s.port > 0 && s.from != ""
}

func (s *Service) SendAccountCreatedEmail(ctx context.Context, toEmail, username, defaultPassword string) error {
	if !s.Enabled() {
		return nil
	}
	_ = ctx

	addr := fmt.Sprintf("%s:%d", s.host, s.port)
	subject := "ELNOTE account created"
	body := fmt.Sprintf("Hello %s,\n\nYour ELNOTE account has been created.\n\nUsername: %s\nTemporary password: %s\n\nPlease sign in and change your password immediately.\n", username, username, defaultPassword)
	message := []byte("From: " + s.from + "\r\n" +
		"To: " + strings.TrimSpace(toEmail) + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n\r\n" +
		body)

	var auth smtp.Auth
	if s.username != "" {
		auth = smtp.PlainAuth("", s.username, s.password, s.host)
	}

	if err := smtp.SendMail(addr, auth, s.from, []string{strings.TrimSpace(toEmail)}, message); err != nil {
		return fmt.Errorf("send mail: %w", err)
	}
	return nil
}
