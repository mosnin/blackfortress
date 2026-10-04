// Package runtime supervises the local Black Fortress stack: PostgreSQL,
// object storage, a mail sink, probod, and bfd's own control API.
package runtime

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"blackfortress.dev/fortress/bfd/internal/guard"
	"blackfortress.dev/fortress/bfd/internal/paths"
	"blackfortress.dev/fortress/bfd/internal/secrets"
)

var Version = "dev"

type Config struct {
	ProbodPort  int
	ControlPort int
	StoragePort int
	MailPort    int
	PgPort      int
	ChromePort  int
}

func DefaultConfig() Config {
	return Config{
		ProbodPort:  envPort("BF_PROBOD_PORT", paths.ProbodPort),
		ControlPort: envPort("BF_CONTROL_PORT", paths.ControlPort),
		StoragePort: envPort("BF_STORAGE_PORT", paths.StoragePort),
		MailPort:    envPort("BF_MAIL_PORT", paths.MailPort),
		PgPort:      envPort("BF_PG_PORT", paths.PgPort),
		ChromePort:  envPort("BF_CHROME_PORT", 7815),
	}
}

func envPort(key string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil && v > 0 {
		return v
	}

	return def
}

type Daemon struct {
	cfg     Config
	layout  paths.Layout
	secrets *secrets.Secrets
	hub     *Hub
	ledger  guard.Ledger
	logger  *log.Logger

	pg      *Postgres
	storage *Storage
	mail    *MailSink
	probod  *Probod

	mu       sync.Mutex
	state    string
	errMsg   string
	services map[string]string
	posture  Posture
	procs    map[string]*Proc

	postureKick chan struct{}
	syncMu      sync.Mutex
	checksMu    sync.Mutex
}

func New(cfg Config, layout paths.Layout, logger *log.Logger) *Daemon {
	return &Daemon{
		cfg:         cfg,
		layout:      layout,
		hub:         NewHub(),
		ledger:      guard.Ledger{Dir: layout.Ledger},
		logger:      logger,
		state:       "starting",
		services:    map[string]string{"postgres": "down", "storage": "down", "mail": "down", "probod": "down", "chrome": "disabled"},
		procs:       map[string]*Proc{},
		postureKick: make(chan struct{}, 1),
	}
}

type Status struct {
	State          string            `json:"state"`
	Error          string            `json:"error,omitempty"`
	Services       map[string]string `json:"services"`
	ConsoleURL     string            `json:"console_url"`
	MCPURL         string            `json:"mcp_url"`
	ControlURL     string            `json:"control_url"`
	OrganizationID string            `json:"organization_id,omitempty"`
	DataDir        string            `json:"data_dir"`
	Version        string            `json:"version"`
}

func (d *Daemon) Status() Status {
	d.mu.Lock()
	defer d.mu.Unlock()

	services := make(map[string]string, len(d.services))
	for k, v := range d.services {
		services[k] = v
	}

	st := Status{
		State:      d.state,
		Error:      d.errMsg,
		Services:   services,
		ConsoleURL: "http://localhost:" + strconv.Itoa(d.cfg.ProbodPort),
		MCPURL:     "http://localhost:" + strconv.Itoa(d.cfg.ControlPort) + "/mcp",
		ControlURL: "http://localhost:" + strconv.Itoa(d.cfg.ControlPort),
		DataDir:    d.layout.Home,
		Version:    Version,
	}
	if d.secrets != nil {
		st.OrganizationID = d.secrets.OrganizationID
	}

	return st
}

func (d *Daemon) probodURL() string {
	return "http://localhost:" + strconv.Itoa(d.cfg.ProbodPort)
}

func (d *Daemon) Posture() Posture {
	d.mu.Lock()
	p := d.posture
	d.mu.Unlock()

	p.Guardrails = d.guardrailSummary()
	if p.Frameworks == nil {
		p.Frameworks = []FrameworkPosture{}
	}

	return p
}

func (d *Daemon) setService(name, state string) {
	d.mu.Lock()
	d.services[name] = state
	d.mu.Unlock()
	d.hub.Publish("status", d.Status())
}

func (d *Daemon) setState(state, errMsg string) {
	d.mu.Lock()
	d.state = state
	d.errMsg = errMsg
	d.mu.Unlock()
	d.hub.Publish("status", d.Status())
}

// Run starts every component and blocks until ctx is cancelled.
func (d *Daemon) Run(ctx context.Context) error {
	if err := d.layout.Ensure(); err != nil {
		return err
	}

	sec, err := secrets.LoadOrCreate(d.layout.Secrets)
	if err != nil {
		return err
	}
	d.secrets = sec

	if err := writeDefaultPolicy(d.layout.Policy); err != nil {
		d.logger.Printf("cannot write policy file: %v", err)
	}

	server := newServer(d)
	if err := server.Start(d.cfg.ControlPort); err != nil {
		return fmt.Errorf("cannot start control API on port %d (is bfd already running?): %w", d.cfg.ControlPort, err)
	}

	err = d.start(ctx)
	if err != nil {
		d.setState("error", err.Error())
		d.logger.Printf("startup failed: %v", err)
	}

	<-ctx.Done()
	d.shutdown()

	return err
}

func (d *Daemon) start(ctx context.Context) error {
	d.storage = &Storage{Dir: d.layout.Objects, Port: d.cfg.StoragePort}
	if err := d.storage.Start(); err != nil {
		return err
	}
	d.setService("storage", "up")

	d.mail = &MailSink{Dir: d.layout.Mail, Port: d.cfg.MailPort}
	if err := d.mail.Start(); err != nil {
		return err
	}
	d.setService("mail", "up")

	pgBin, err := paths.FindPostgresBinDir()
	if err != nil {
		return err
	}

	d.pg = &Postgres{
		BinDir:   pgBin,
		DataDir:  d.layout.PgData,
		RunDir:   d.layout.PgRun,
		Port:     d.cfg.PgPort,
		Password: d.secrets.PgPassword,
		LogPath:  filepath.Join(d.layout.Logs, "postgres.log"),
	}

	d.setService("postgres", "starting")
	if err := d.pg.Init(ctx); err != nil {
		return err
	}

	pgProc, err := d.pg.Start(ctx)
	if err != nil {
		return err
	}
	d.track("postgres", pgProc)
	d.setService("postgres", "up")

	chromeAddr := d.startChrome()

	probodBin, err := paths.FindBinary("probod")
	if err != nil {
		return err
	}

	bootstrapBin, err := paths.FindBinary("probod-bootstrap")
	if err != nil {
		return err
	}

	d.probod = &Probod{
		Bin:          probodBin,
		BootstrapBin: bootstrapBin,
		ConfigPath:   d.layout.Config,
		LogPath:      filepath.Join(d.layout.Logs, "probod.log"),
		Port:         d.cfg.ProbodPort,
		ControlPort:  d.cfg.ControlPort,
		PgAddr:       d.pg.Addr(),
		StorageURL:   d.storage.Endpoint(),
		MailAddr:     d.mail.Addr(),
		ChromeAddr:   chromeAddr,
		Secrets:      d.secrets,
		// Sign-up stays open only until the local owner exists.
		DisableSignup: d.secrets.OrganizationID != "",
	}

	if err := d.startProbod(ctx); err != nil {
		return err
	}

	d.setState("provisioning", "")

	save := func() error { return d.secrets.Save(d.layout.Secrets) }
	sess, err := Provision(ctx, d.probod.BaseURL(), d.layout.Mail, d.secrets, save)
	if err != nil {
		return err
	}

	if !d.probod.DisableSignup {
		d.logger.Printf("local owner provisioned; closing sign-up")
		d.probod.DisableSignup = true
		d.stopProc("probod", syscall.SIGTERM)

		if err := d.startProbod(ctx); err != nil {
			return err
		}
	}

	importDefaultFrameworks(ctx, sess, d.secrets.OrganizationID, d.logger.Printf)

	d.setState("running", "")
	go d.supervise(ctx)
	go d.postureLoop(ctx)
	go d.evidenceLoop(ctx)
	go d.checksLoop(ctx)

	return nil
}

func (d *Daemon) startProbod(ctx context.Context) error {
	d.setService("probod", "starting")

	if err := d.probod.WriteConfig(ctx); err != nil {
		return err
	}

	proc, err := d.probod.Start(ctx)
	if err != nil {
		d.setService("probod", "down")
		return err
	}

	d.track("probod", proc)
	d.setService("probod", "up")

	return nil
}

func (d *Daemon) track(name string, p *Proc) {
	d.mu.Lock()
	d.procs[name] = p
	d.mu.Unlock()
}

func (d *Daemon) stopProc(name string, sig syscall.Signal) {
	d.mu.Lock()
	p := d.procs[name]
	delete(d.procs, name)
	d.mu.Unlock()

	if p != nil {
		p.Stop(sig, 20*time.Second)
	}
}

// supervise restarts probod or postgres if they exit unexpectedly.
func (d *Daemon) supervise(ctx context.Context) {
	backoff := time.Second

	for {
		d.mu.Lock()
		pg, pd := d.procs["postgres"], d.procs["probod"]
		d.mu.Unlock()

		if pg == nil || pd == nil {
			return
		}

		select {
		case <-ctx.Done():
			return
		case <-pg.Done():
			d.logger.Printf("postgres exited; restarting")
			d.setService("postgres", "restarting")
			d.setState("degraded", "postgres exited")
			d.stopProc("probod", syscall.SIGTERM)

			if p, err := d.pg.Start(ctx); err == nil {
				d.track("postgres", p)
				d.setService("postgres", "up")
			} else {
				d.logger.Printf("postgres restart failed: %v", err)
			}
		case <-pd.Done():
			d.logger.Printf("probod exited; restarting")
			d.setService("probod", "restarting")
			d.setState("degraded", "probod exited")
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		if backoff < time.Minute {
			backoff *= 2
		}

		if err := d.startProbod(ctx); err != nil {
			d.logger.Printf("probod restart failed: %v", err)
			continue
		}

		backoff = time.Second
		d.setState("running", "")
	}
}

func (d *Daemon) postureLoop(ctx context.Context) {
	refresh := func() {
		fw, err := FetchPosture(ctx, d.probod.BaseURL(), d.secrets.APIKey, d.secrets.OrganizationID)
		if err != nil {
			d.logger.Printf("posture refresh failed: %v", err)
			return
		}

		d.mu.Lock()
		d.posture = Posture{UpdatedAt: time.Now().UTC(), Frameworks: fw}
		d.mu.Unlock()
		d.hub.Publish("posture", d.Posture())
	}

	refresh()

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			refresh()
		case <-d.postureKick:
			// Coalesce bursts of agent activity into one refresh.
			time.Sleep(2 * time.Second)
			refresh()
		}
	}
}

// evidenceLoop uploads completed days of agent evidence to Probo at startup
// and then hourly.
func (d *Daemon) evidenceLoop(ctx context.Context) {
	run := func() {
		n, err := d.SyncEvidence(ctx, false)
		if err != nil {
			d.logger.Printf("evidence sync failed: %v", err)
			return
		}

		if n > 0 {
			d.logger.Printf("uploaded %d evidence reports", n)
		}
	}

	run()

	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}

func (d *Daemon) kickPosture() {
	select {
	case d.postureKick <- struct{}{}:
	default:
	}
}

func (d *Daemon) publishEntry(e guard.Entry) {
	typ := "evidence"
	if e.Decision == guard.ActionBlock || e.Decision == guard.ActionAsk {
		typ = "guardrail"
	}

	d.hub.Publish(typ, e)
	d.kickPosture()
}

// recordMCPCalls writes one ledger entry per MCP tool call an agent makes.
func (d *Daemon) recordMCPCalls(userAgent string, body []byte) {
	for _, c := range parseRPCCalls(body) {
		if c.Method != "tools/call" {
			continue
		}

		e := guard.Entry{
			Agent:    agentFromUserAgent(userAgent),
			Event:    "MCPToolCall",
			Tool:     "mcp:" + c.Params.Name,
			Target:   summarizeArgs(c.Params.Arguments),
			Decision: guard.ActionRecord,
			Controls: mcpControls(c.Params.Name),
		}

		if err := d.ledger.Append(&e); err != nil {
			d.logger.Printf("cannot record MCP call: %v", err)
			continue
		}

		d.publishEntry(e)
	}
}

func agentFromUserAgent(ua string) string {
	switch l := strings.ToLower(ua); {
	case strings.Contains(l, "claude"):
		return "claude-code"
	case strings.Contains(l, "cursor"):
		return "cursor"
	case strings.Contains(l, "codex"):
		return "codex"
	case ua == "":
		return "mcp-client"
	default:
		if i := strings.IndexAny(ua, "/ "); i > 0 {
			return ua[:i]
		}

		return ua
	}
}

func summarizeArgs(raw []byte) string {
	s := strings.Join(strings.Fields(string(raw)), " ")
	if len(s) > 200 {
		s = s[:200] + "…"
	}

	return s
}

// mcpControls tags compliance-record changes made by agents: writes to the
// GRC record itself fall under documented information and change control.
func mcpControls(tool string) []string {
	l := strings.ToLower(tool)
	if strings.HasPrefix(l, "list") || strings.HasPrefix(l, "get") || strings.HasPrefix(l, "search") {
		return nil
	}

	return []string{"ISO27001:7.5.3", "SOC2:CC2.1"}
}

func (d *Daemon) guardrailSummary() GuardrailSummary {
	entries, err := d.ledger.Recent(5000)
	if err != nil {
		return GuardrailSummary{}
	}

	var g GuardrailSummary
	cutoff := time.Now().Add(-24 * time.Hour)

	for _, e := range entries {
		if e.Time.Before(cutoff) {
			break
		}

		g.Last24h++
		switch e.Decision {
		case guard.ActionBlock:
			g.Blocked++
		case guard.ActionAsk:
			g.Asked++
		case guard.ActionRecord:
			g.Recorded++
		}
	}

	return g
}

// startChrome launches a headless Chrome for PDF export when one is
// installed. PDF export is the only feature that needs it.
func (d *Daemon) startChrome() string {
	bin := findChrome()
	if bin == "" {
		return ""
	}

	cmd := exec.Command(
		bin,
		"--headless=new",
		"--disable-gpu",
		"--no-first-run",
		"--no-default-browser-check",
		"--remote-debugging-address=127.0.0.1",
		"--remote-debugging-port="+strconv.Itoa(d.cfg.ChromePort),
		"--user-data-dir="+filepath.Join(d.layout.Home, "chrome"),
	)

	p, err := startProc("chrome", cmd, filepath.Join(d.layout.Logs, "chrome.log"))
	if err != nil {
		d.logger.Printf("chrome unavailable: %v", err)
		return ""
	}

	d.track("chrome", p)
	d.setService("chrome", "up")

	return "127.0.0.1:" + strconv.Itoa(d.cfg.ChromePort)
}

func findChrome() string {
	if v := os.Getenv("BF_CHROME"); v != "" {
		return v
	}

	candidates := []string{"google-chrome", "chromium", "chromium-browser"}
	if runtime.GOOS == "darwin" {
		candidates = append([]string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
		}, candidates...)
	}

	for _, c := range candidates {
		if filepath.IsAbs(c) {
			if _, err := os.Stat(c); err == nil {
				return c
			}

			continue
		}

		if p, err := exec.LookPath(c); err == nil {
			return p
		}
	}

	return ""
}

func (d *Daemon) shutdown() {
	d.setState("stopping", "")
	d.stopProc("probod", syscall.SIGTERM)
	d.stopProc("chrome", syscall.SIGTERM)
	// SIGINT is postgres "fast" shutdown: it rolls back open transactions
	// and checkpoints, unlike SIGTERM's "smart" wait for clients.
	d.stopProc("postgres", syscall.SIGINT)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if d.storage != nil {
		d.storage.Stop(ctx)
	}

	if d.mail != nil {
		d.mail.Stop()
	}
}

func writeDefaultPolicy(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	// The user file starts empty: built-in rules apply, and any rule added
	// here with the same id overrides the default.
	const starter = `{
  "version": 1,
  "rules": []
}
`

	return os.WriteFile(path, []byte(starter), 0o600)
}
