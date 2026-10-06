package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"

	"blackfortress.dev/fortress/bfd/internal/guard"
	"blackfortress.dev/fortress/bfd/internal/paths"
)

// Outcome, CheckReport and ProviderRun mirror bf-checks' JSON protocol
// (fortress/checks/src/protocol.ts).
type Outcome struct {
	Title        string `json:"title"`
	Description  string `json:"description,omitempty"`
	ResourceType string `json:"resourceType"`
	ResourceID   string `json:"resourceId"`
	Severity     string `json:"severity,omitempty"`
	Remediation  string `json:"remediation,omitempty"`
	Evidence     any    `json:"evidence,omitempty"`
}

type CheckReport struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	TaskMapping string    `json:"taskMapping,omitempty"`
	Status      string    `json:"status"`
	Error       string    `json:"error,omitempty"`
	DurationMs  int64     `json:"durationMs"`
	Passed      []Outcome `json:"passed"`
	Findings    []Outcome `json:"findings"`
}

// Verdict is how Black Fortress reads a check result. Comp reports
// "success" whenever there are no findings, including when a check found
// nothing to evaluate; that is inconclusive, not evidence of compliance.
func (c CheckReport) Verdict() string {
	switch {
	case c.Status == "error":
		return "error"
	case len(c.Findings) > 0:
		return "fail"
	case len(c.Passed) > 0:
		return "pass"
	default:
		return "inconclusive"
	}
}

type ProviderRun struct {
	Provider     string        `json:"provider"`
	ProviderName string        `json:"providerName"`
	RanAt        time.Time     `json:"ranAt"`
	DurationMs   int64         `json:"durationMs"`
	Checks       []CheckReport `json:"checks"`
	Source       string        `json:"source,omitempty"`
	Error        string        `json:"error,omitempty"`
}

// ChecksState is $BF_HOME/checks/latest.json and the /v1/checks payload.
type ChecksState struct {
	UpdatedAt time.Time               `json:"updated_at"`
	Providers map[string]*ProviderRun `json:"providers"`
	Skipped   map[string]string       `json:"skipped"`
}

// checkProviders lists the providers bf-checks can run, in run order.
var checkProviders = []string{"github", "aws", "gcp", "azure", "vercel", "google-workspace", "aikido"}

func (d *Daemon) checksDir() string { return filepath.Join(d.layout.Home, "checks") }

func (d *Daemon) loadChecksState() *ChecksState {
	st := &ChecksState{Providers: map[string]*ProviderRun{}, Skipped: map[string]string{}}

	if data, err := os.ReadFile(filepath.Join(d.checksDir(), "latest.json")); err == nil {
		_ = json.Unmarshal(data, st)
	}

	if st.Providers == nil {
		st.Providers = map[string]*ProviderRun{}
	}

	if st.Skipped == nil {
		st.Skipped = map[string]string{}
	}

	return st
}

func (d *Daemon) saveChecksState(st *ChecksState) error {
	if err := os.MkdirAll(d.checksDir(), 0o700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(d.checksDir(), "latest.json"), data, 0o600)
}

// RunChecks runs every enabled provider with local credentials (or only
// `only`), records results in the ledger, and pushes them to Probo.
func (d *Daemon) RunChecks(ctx context.Context, only string) (*ChecksState, error) {
	d.checksMu.Lock()
	defer d.checksMu.Unlock()

	bin, err := paths.FindBinary("bf-checks")
	if err != nil {
		return nil, err
	}

	cfg, err := loadChecksConfig(filepath.Join(d.layout.Home, "checks.json"))
	if err != nil {
		return nil, err
	}

	st := d.loadChecksState()
	ran := map[string]bool{}

	for _, provider := range checkProviders {
		if only != "" && provider != only {
			continue
		}

		pcfg := cfg.Providers[provider]
		if !pcfg.enabled() {
			st.Skipped[provider] = "disabled in checks.json"
			continue
		}

		auth, err := discoverAuth(ctx, provider, pcfg, d.layout.Ledger)
		if errors.Is(err, errNotConfigured) {
			st.Skipped[provider] = "no local credentials"
			continue
		}

		if err != nil {
			st.Skipped[provider] = err.Error()
			continue
		}

		delete(st.Skipped, provider)

		run, err := execChecks(ctx, bin, provider, auth)
		if err != nil {
			run = &ProviderRun{Provider: provider, ProviderName: provider, RanAt: time.Now().UTC(), Error: err.Error()}
		}

		run.Source = auth.Source
		st.Providers[provider] = run
		ran[provider] = true
		d.recordCheckRun(run)
	}

	st.UpdatedAt = time.Now().UTC()
	if err := d.saveChecksState(st); err != nil {
		return st, err
	}

	d.hub.Publish("checks", st.Summary())

	if err := d.syncCheckEvidence(ctx, st, ran); err != nil {
		d.logger.Printf("check evidence sync failed: %v", err)
	}

	d.kickPosture()

	return st, nil
}

func execChecks(ctx context.Context, bin, provider string, auth *providerAuth) (*ProviderRun, error) {
	req, err := json.Marshal(map[string]any{
		"provider":    provider,
		"accessToken": auth.AccessToken,
		"credentials": auth.Credentials,
		"variables":   auth.Variables,
	})
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()

	var stdout, stderr bytes.Buffer

	cmd := exec.CommandContext(ctx, bin, "run")
	cmd.Env = checksEnv()
	cmd.Stdin = bytes.NewReader(req)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("bf-checks %s: %w: %s", provider, err, lastLine(stderr.String()))
	}

	var run ProviderRun
	if err := json.Unmarshal(stdout.Bytes(), &run); err != nil {
		return nil, fmt.Errorf("bf-checks %s: invalid output: %w", provider, err)
	}

	return &run, nil
}

// checksEnv is the environment bf-checks runs with: enough to reach the
// network (proxies, CA bundles) but none of bfd's own or other providers'
// credentials, which travel only in the JSON request.
func checksEnv() []string {
	env := minimalEnv()

	for _, k := range []string{
		"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "NO_PROXY", "no_proxy",
		"SSL_CERT_FILE", "SSL_CERT_DIR", "NODE_EXTRA_CA_CERTS", "AWS_CA_BUNDLE",
	} {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}

	return env
}

func lastLine(s string) string {
	lines := bytes.Split(bytes.TrimSpace([]byte(s)), []byte("\n"))
	return string(lines[len(lines)-1])
}

// recordCheckRun writes one ledger entry per check so automated evidence
// shares the tamper-evident trail with agent activity.
func (d *Daemon) recordCheckRun(run *ProviderRun) {
	for _, c := range run.Checks {
		outcome := c.Verdict()
		switch outcome {
		case "fail":
			outcome = fmt.Sprintf("fail (%d findings)", len(c.Findings))
		case "error":
			outcome = "error: " + c.Error
		}

		e := guard.Entry{
			Agent:    "bf-checks",
			Event:    "CheckRun",
			Tool:     "check:" + run.Provider + "/" + c.ID,
			Target:   c.Name,
			Decision: guard.ActionRecord,
			Outcome:  outcome,
		}

		if err := d.ledger.Append(&e); err != nil {
			d.logger.Printf("cannot record check run: %v", err)
			continue
		}

		d.hub.Publish("evidence", e)
	}
}

type ChecksSummary struct {
	UpdatedAt time.Time         `json:"updated_at"`
	Providers []ProviderSummary `json:"providers"`
	Skipped   map[string]string `json:"skipped"`
}

type ProviderSummary struct {
	Provider     string    `json:"provider"`
	Name         string    `json:"name"`
	RanAt        time.Time `json:"ran_at"`
	Source       string    `json:"source,omitempty"`
	Error        string    `json:"error,omitempty"`
	Passing      int       `json:"checks_passing"`
	Failing      int       `json:"checks_failing"`
	Inconclusive int       `json:"checks_inconclusive"`
	Errored      int       `json:"checks_errored"`
	Findings     int       `json:"findings"`
}

func (st *ChecksState) Summary() ChecksSummary {
	s := ChecksSummary{UpdatedAt: st.UpdatedAt, Skipped: st.Skipped, Providers: []ProviderSummary{}}

	for _, run := range st.Providers {
		ps := ProviderSummary{Provider: run.Provider, Name: run.ProviderName, RanAt: run.RanAt, Source: run.Source, Error: run.Error}

		for _, c := range run.Checks {
			switch c.Verdict() {
			case "pass":
				ps.Passing++
			case "fail":
				ps.Failing++
			case "inconclusive":
				ps.Inconclusive++
			default:
				ps.Errored++
			}

			ps.Findings += len(c.Findings)
		}

		s.Providers = append(s.Providers, ps)
	}

	sort.Slice(s.Providers, func(i, j int) bool { return s.Providers[i].Provider < s.Providers[j].Provider })

	return s
}

// checksLoop runs checks shortly after startup, then on the configured
// interval.
func (d *Daemon) checksLoop(ctx context.Context) {
	if _, err := paths.FindBinary("bf-checks"); err != nil {
		d.logger.Printf("automated checks disabled: %v", err)
		return
	}

	select {
	case <-ctx.Done():
		return
	case <-time.After(30 * time.Second):
	}

	for {
		if _, err := d.RunChecks(ctx, ""); err != nil {
			d.logger.Printf("automated checks failed: %v", err)
		}

		interval := 6 * time.Hour
		if cfg, err := loadChecksConfig(filepath.Join(d.layout.Home, "checks.json")); err == nil {
			interval = time.Duration(cfg.IntervalMinutes) * time.Minute
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}
