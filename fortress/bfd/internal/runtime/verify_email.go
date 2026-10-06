package runtime

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var verifyTokenRe = regexp.MustCompile(`verify-email\?token=([A-Za-z0-9._~%-]+)`)

// waitForVerificationToken scans captured mail for the newest email
// confirmation link sent after since. probod only verifies addresses by
// email, and every outbound message lands in the local mail sink.
func waitForVerificationToken(ctx context.Context, dir string, since time.Time, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if tok := newestToken(dir, since); tok != "" {
			return tok, nil
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(time.Second):
		}
	}

	return "", errors.New("no verification email arrived")
}

func newestToken(dir string, since time.Time) string {
	files, _ := filepath.Glob(filepath.Join(dir, "*.eml"))
	sort.Sort(sort.Reverse(sort.StringSlice(files)))

	for _, f := range files {
		info, err := os.Stat(f)
		if err != nil || info.ModTime().Before(since) {
			continue
		}

		raw, err := os.ReadFile(f)
		if err != nil {
			continue
		}

		if m := verifyTokenRe.FindSubmatch(decodedBody(raw)); m != nil {
			return string(m[1])
		}
	}

	return ""
}

// decodedBody returns the message text with transfer encodings undone, so
// a quoted-printable soft line break cannot split the token.
func decodedBody(raw []byte) []byte {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return raw
	}

	var out bytes.Buffer
	collectParts(msg.Header.Get("Content-Type"), msg.Header.Get("Content-Transfer-Encoding"), msg.Body, &out)

	return out.Bytes()
}

func collectParts(contentType, encoding string, body io.Reader, out *bytes.Buffer) {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err == nil && strings.HasPrefix(mediaType, "multipart/") {
		mr := multipart.NewReader(body, params["boundary"])
		for {
			p, err := mr.NextRawPart()
			if err != nil {
				return
			}

			collectParts(p.Header.Get("Content-Type"), p.Header.Get("Content-Transfer-Encoding"), p, out)
		}
	}

	if strings.EqualFold(encoding, "quoted-printable") {
		body = quotedprintable.NewReader(body)
	}

	_, _ = io.Copy(out, body)
	out.WriteByte('\n')
}
