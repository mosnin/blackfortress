// Package runtime supervises the local Black Fortress stack: PostgreSQL,
// object storage, a mail sink, probod, and bfd's own control API.
package runtime

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"

	"blackfortress.dev/fortress/bfd/internal/guard"
	"blackfortress.dev/fortress/bfd/internal/paths"
	"blackfortress.dev/fortress/bfd/internal/secrets"
)

var Version = "dev"

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
	d.ledger.Key = sec.LedgerKeyBytes()

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
	// A previous bfd that crashed or was killed leaves its children running
	// in their own process groups; stop them before taking their ports.
	killOrphans(d.runDir(), d.layout.PgData, d.logger.Printf)

	d.storage = &Storage{
		Dir:           d.layout.Objects,
		Port:          d.cfg.StoragePort,
		KeyID:         d.secrets.StorageKeyID,
		ConsoleOrigin: d.probodURL(),
	}
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

	pgCred, err := pgCredential()
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
		Cred:     pgCred,
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

	if err := d.importPolicies(ctx, sess); err != nil {
		d.logger.Printf("policy import incomplete (retried next start): %v", err)
	}

	// Have a posture before announcing "running", so a client that asks
	// right away (bf up && bf report) does not see an empty one.
	d.refreshPosture(ctx)

	d.setState("running", "")
	go d.supervise(ctx)
	go d.postureLoop(ctx)
	go d.evidenceLoop(ctx)
	go d.checksLoop(ctx)

	return nil
}
