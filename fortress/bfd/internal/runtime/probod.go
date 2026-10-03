package runtime

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"

	"blackfortress.dev/fortress/bfd/internal/secrets"
)

// Probod runs the Probo server with a configuration generated for a single
// local user: everything on loopback, signup closed after provisioning,
// mail and storage pointed at bfd's in-process sinks.
type Probod struct {
	Bin           string
	BootstrapBin  string
	ConfigPath    string
	LogPath       string
	Port          int
	ControlPort   int
	PgAddr        string
	StorageURL    string
	MailAddr      string
	ChromeAddr    string
	Secrets       *secrets.Secrets
	DisableSignup bool
	ExtraEnv      []string
}

func (p *Probod) BaseURL() string { return "http://localhost:" + strconv.Itoa(p.Port) }

func (p *Probod) env() []string {
	s := p.Secrets
	env := []string{
		"PROBOD_BASE_URL=" + p.BaseURL(),
		"PROBOD_API_ADDR=127.0.0.1:" + strconv.Itoa(p.Port),
		"PROBOD_API_CORS_ALLOWED_ORIGINS=" + p.BaseURL() + ",http://localhost:" + strconv.Itoa(p.ControlPort),
		"PROBOD_BRANDING=Black Fortress",
		"PROBOD_ENCRYPTION_KEY=" + s.EncryptionKey,
		"PROBOD_AUTH_COOKIE_NAME=BFSID",
		"PROBOD_AUTH_COOKIE_SECRET=" + s.CookieSecret,
		"PROBOD_AUTH_COOKIE_SECURE=false",
		"PROBOD_AUTH_PASSWORD_PEPPER=" + s.PasswordPepper,
		"PROBOD_AUTH_DISABLE_SIGNUP=" + strconv.FormatBool(p.DisableSignup),
		"PROBOD_OAUTH2_SERVER_SIGNING_KEY=" + s.OAuth2SigningKey,
		"PROBOD_PG_ADDR=" + p.PgAddr,
		"PROBOD_PG_USERNAME=" + pgUser,
		"PROBOD_PG_PASSWORD=" + s.PgPassword,
		"PROBOD_PG_DATABASE=" + pgDatabase,
		"PROBOD_PG_POOL_SIZE=20",
		"PROBOD_AWS_REGION=" + storageRegion,
		"PROBOD_AWS_BUCKET=" + storageBucket,
		"PROBOD_AWS_ACCESS_KEY_ID=" + storageAccessKey,
		"PROBOD_AWS_SECRET_ACCESS_KEY=" + storageAccessKey,
		"PROBOD_AWS_ENDPOINT=" + p.StorageURL,
		"PROBOD_AWS_USE_PATH_STYLE=true",
		"PROBOD_SMTP_ADDR=" + p.MailAddr,
		"PROBOD_SMTP_TLS_REQUIRED=false",
		"PROBOD_MAILER_SENDER_NAME=Black Fortress",
		"PROBOD_MAILER_SENDER_EMAIL=no-reply@blackfortress.local",
		"PROBOD_MAILER_INTERVAL=2",
		"PROBOD_TRACING_ADDR=",
		"PROBOD_METRICS_ADDR=127.0.0.1:0",
		"PROBOD_TRUST_CENTER_HTTP_ADDR=127.0.0.1:0",
		"PROBOD_TRUST_CENTER_HTTPS_ADDR=127.0.0.1:0",
		"PROBOD_TRUST_CENTER_BASE_DOMAIN=trust.localhost",
		"PROBOD_TRUST_CENTER_TLS_MODE=external",
	}

	if p.ChromeAddr != "" {
		env = append(env, "PROBOD_CHROME_DP_ADDR="+p.ChromeAddr)
	}

	// LLM keys pass through from the user's environment so AI features
	// (vendor vetting, evidence description) work when configured.
	haveLLM := false
	for _, k := range []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "FIRECRAWL_API_KEY"} {
		if v := os.Getenv("BF_" + k); v != "" {
			env = append(env, "PROBOD_"+k+"="+v)
			haveLLM = haveLLM || k != "FIRECRAWL_API_KEY"
		}
	}

	// probod refuses to start without an LLM provider. Register a
	// placeholder so everything else works offline; AI features return
	// provider errors until a real key is configured.
	if !haveLLM {
		env = append(env, "PROBOD_OPENAI_API_KEY=not-configured")
	} else if os.Getenv("BF_OPENAI_API_KEY") == "" {
		env = append(env, "PROBOD_AGENT_DEFAULT_PROVIDER=anthropic", "PROBOD_AGENT_DEFAULT_MODEL_NAME=claude-sonnet-5-5")
	}

	return append(env, p.ExtraEnv...)
}

// WriteConfig regenerates probod's config file through probod-bootstrap so
// the format always matches the bundled probod version.
func (p *Probod) WriteConfig(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, p.BootstrapBin, "-output", p.ConfigPath)
	cmd.Env = append(minimalEnv(), p.env()...)

	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("probod-bootstrap failed: %w\n%s", err, out)
	}

	return os.Chmod(p.ConfigPath, 0o600)
}

func (p *Probod) Start(ctx context.Context) (*Proc, error) {
	cmd := exec.Command(p.Bin, "-cfg-file", p.ConfigPath)
	cmd.Env = minimalEnv()

	proc, err := startProc("probod", cmd, p.LogPath)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(3 * time.Minute)

	for time.Now().Before(deadline) {
		if resp, err := client.Get(p.BaseURL() + "/.well-known/openid-configuration"); err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return proc, nil
			}
		}

		select {
		case <-proc.Done():
			return nil, errors.New("probod exited during startup; see " + p.LogPath)
		case <-ctx.Done():
			proc.Stop(syscall.SIGTERM, 15*time.Second)
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}

	proc.Stop(syscall.SIGTERM, 15*time.Second)

	return nil, errors.New("probod did not become ready in time; see " + p.LogPath)
}

func minimalEnv() []string {
	var env []string
	for _, k := range []string{"HOME", "PATH", "TMPDIR", "USER", "LANG"} {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}

	return env
}
