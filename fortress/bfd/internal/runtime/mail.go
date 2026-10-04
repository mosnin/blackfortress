package runtime

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-smtp"
)

// MailSink accepts probod's outbound mail on loopback and stores each
// message as an .eml file, so nothing leaves the machine.
type MailSink struct {
	Dir  string
	Port int

	srv *smtp.Server
}

func (m *MailSink) Addr() string { return "127.0.0.1:" + strconv.Itoa(m.Port) }

func (m *MailSink) Start() error {
	s := smtp.NewServer(&mailBackend{dir: m.Dir})
	s.Domain = "localhost"
	s.ReadTimeout = 30 * time.Second
	s.WriteTimeout = 30 * time.Second
	s.MaxMessageBytes = 25 << 20
	s.AllowInsecureAuth = true

	ln, err := net.Listen("tcp", m.Addr())
	if err != nil {
		return fmt.Errorf("cannot listen for mail: %w", err)
	}

	m.srv = s
	go func() { _ = s.Serve(ln) }()

	return nil
}

func (m *MailSink) Stop() {
	if m.srv != nil {
		_ = m.srv.Close()
	}
}

type mailBackend struct{ dir string }

func (b *mailBackend) NewSession(_ *smtp.Conn) (smtp.Session, error) {
	return &mailSession{dir: b.dir}, nil
}

type mailSession struct {
	dir string
	to  []string
}

// Mail accepts only probod's configured sender, so other local software
// (or a browser cross-protocol request) cannot drop messages into the
// sink, which bfd reads during first-run email verification.
func (s *mailSession) Mail(from string, _ *smtp.MailOptions) error {
	if !strings.EqualFold(from, mailSender) {
		return &smtp.SMTPError{Code: 550, Message: "sender not accepted"}
	}

	return nil
}

func (s *mailSession) Rcpt(to string, _ *smtp.RcptOptions) error {
	s.to = append(s.to, to)
	return nil
}

func (s *mailSession) Data(r io.Reader) error {
	name := fmt.Sprintf("%s.eml", time.Now().UTC().Format("20060102T150405.000000000"))

	f, err := os.OpenFile(filepath.Join(s.dir, name), os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, r)

	return err
}

func (s *mailSession) Reset()        { s.to = nil }
func (s *mailSession) Logout() error { return nil }
