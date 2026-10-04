package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ChecksConfig is $BF_HOME/checks.json. Providers are enabled
// automatically when local credentials are found; the file only needs
// editing to disable one or to set variables (repositories, projects).
type ChecksConfig struct {
	IntervalMinutes int                       `json:"interval_minutes"`
	Providers       map[string]ProviderConfig `json:"providers"`
}

type ProviderConfig struct {
	Enabled   *bool          `json:"enabled,omitempty"`
	Variables map[string]any `json:"variables,omitempty"`
	// Regions (AWS) to scan; defaults to the CLI's configured region.
	Regions []string `json:"regions,omitempty"`
	// Profile (AWS) to read credentials from; defaults to the CLI default.
	Profile string `json:"profile,omitempty"`
}

func (p ProviderConfig) enabled() bool { return p.Enabled == nil || *p.Enabled }

func loadChecksConfig(path string) (*ChecksConfig, error) {
	cfg := &ChecksConfig{IntervalMinutes: 360, Providers: map[string]ProviderConfig{}}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		out, _ := json.MarshalIndent(cfg, "", "  ")
		_ = os.WriteFile(path, append(out, '\n'), 0o600)

		return cfg, nil
	}

	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	if cfg.IntervalMinutes <= 0 {
		cfg.IntervalMinutes = 360
	}

	if cfg.Providers == nil {
		cfg.Providers = map[string]ProviderConfig{}
	}

	return cfg, nil
}

// providerAuth is what bf-checks needs to run one provider.
type providerAuth struct {
	AccessToken string
	Credentials map[string]any
	Variables   map[string]any
	Source      string
}

// errNotConfigured means no local credentials exist for a provider; the
// provider is skipped silently rather than reported as failing.
var errNotConfigured = errors.New("no local credentials")

// discoverAuth finds credentials for provider from the environment or the
// provider's own CLI, the way the developer already authenticates.
func discoverAuth(ctx context.Context, provider string, cfg ProviderConfig, ledgerDir string) (*providerAuth, error) {
	auth := &providerAuth{Credentials: map[string]any{}, Variables: map[string]any{}}

	switch provider {
	case "github":
		auth.AccessToken, auth.Source = firstEnv("BF_GITHUB_TOKEN", "GH_TOKEN", "GITHUB_TOKEN")
		if auth.AccessToken == "" {
			auth.AccessToken, auth.Source = cliOutput(ctx, "gh", "auth", "token")
		}

		if repos := githubReposFromLedger(ledgerDir); len(repos) > 0 {
			auth.Variables["target_repos"] = repos
		}
	case "aws":
		if err := awsAuth(ctx, cfg, auth); err != nil {
			return nil, err
		}
	case "gcp":
		auth.AccessToken, auth.Source = firstEnv("BF_GCP_TOKEN")
		if auth.AccessToken == "" {
			auth.AccessToken, auth.Source = cliOutput(ctx, "gcloud", "auth", "print-access-token")
		}

		if project, _ := cliOutput(ctx, "gcloud", "config", "get-value", "project"); project != "" && project != "(unset)" {
			auth.Variables["project_ids"] = []string{project}
		}
	case "azure":
		auth.AccessToken, auth.Source = firstEnv("BF_AZURE_TOKEN")
		if auth.AccessToken == "" {
			auth.AccessToken, auth.Source = cliOutput(ctx, "az", "account", "get-access-token",
				"--resource", "https://management.azure.com", "--query", "accessToken", "-o", "tsv")
		}

		if sub, _ := cliOutput(ctx, "az", "account", "show", "--query", "id", "-o", "tsv"); sub != "" {
			auth.Variables["subscription_id"] = sub
		}
	case "vercel":
		auth.AccessToken, auth.Source = firstEnv("BF_VERCEL_TOKEN", "VERCEL_TOKEN")
	case "google-workspace":
		// gcloud user tokens lack the Admin SDK scopes these checks need.
		auth.AccessToken, auth.Source = firstEnv("BF_GOOGLE_WORKSPACE_TOKEN")
	case "aikido":
		auth.AccessToken, auth.Source = firstEnv("BF_AIKIDO_TOKEN")
	default:
		return nil, errNotConfigured
	}

	if auth.AccessToken == "" && len(auth.Credentials) == 0 {
		return nil, errNotConfigured
	}

	for k, v := range cfg.Variables {
		auth.Variables[k] = v
	}

	return auth, nil
}

func awsAuth(ctx context.Context, cfg ProviderConfig, auth *providerAuth) error {
	key, src := firstEnv("AWS_ACCESS_KEY_ID")
	secret := os.Getenv("AWS_SECRET_ACCESS_KEY")
	session := os.Getenv("AWS_SESSION_TOKEN")

	if key == "" || secret == "" {
		args := []string{"configure", "export-credentials", "--format", "env-no-export"}
		if cfg.Profile != "" {
			args = append(args, "--profile", cfg.Profile)
		}

		out, s := cliOutput(ctx, "aws", args...)
		vals := parseEnvLines(out)
		key, secret, session, src = vals["AWS_ACCESS_KEY_ID"], vals["AWS_SECRET_ACCESS_KEY"], vals["AWS_SESSION_TOKEN"], s
	}

	if key == "" || secret == "" {
		return errNotConfigured
	}

	regions := cfg.Regions
	if len(regions) == 0 {
		if r, _ := firstEnv("AWS_REGION", "AWS_DEFAULT_REGION"); r != "" {
			regions = []string{r}
		} else if r, _ := cliOutput(ctx, "aws", "configure", "get", "region"); r != "" {
			regions = []string{r}
		} else {
			regions = []string{"us-east-1"}
		}
	}

	auth.Source = src
	auth.Credentials = map[string]any{
		"access_key_id":     key,
		"secret_access_key": secret,
		"regions":           regions,
	}

	if session != "" {
		auth.Credentials["session_token"] = session
	}

	return nil
}

func firstEnv(keys ...string) (string, string) {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v, "env:" + k
		}
	}

	return "", ""
}

// cliOutput runs a provider CLI if it is installed, returning trimmed
// stdout, or "" when the CLI is missing or not logged in.
func cliOutput(ctx context.Context, name string, args ...string) (string, string) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", ""
	}

	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, path, args...).Output()
	if err != nil {
		return "", ""
	}

	return strings.TrimSpace(string(out)), "cli:" + name
}

func parseEnvLines(s string) map[string]string {
	vals := map[string]string{}

	for _, line := range strings.Split(s, "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
			vals[k] = strings.Trim(v, `"'`)
		}
	}

	return vals
}

// githubReposFromLedger returns owner/repo for every GitHub repository an
// agent has worked in: the repositories this developer actually changes.
func githubReposFromLedger(ledgerDir string) []string {
	roots := map[string]bool{}

	files, _ := filepath.Glob(filepath.Join(ledgerDir, "*.jsonl"))
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}

		for _, line := range strings.Split(string(data), "\n") {
			var e struct {
				Repo string `json:"repo"`
			}
			if json.Unmarshal([]byte(line), &e) == nil && e.Repo != "" {
				roots[e.Repo] = true
			}
		}
	}

	seen := map[string]bool{}

	var repos []string

	for root := range roots {
		out, err := exec.Command("git", "-C", root, "remote", "get-url", "origin").Output()
		if err != nil {
			continue
		}

		if slug := githubSlug(strings.TrimSpace(string(out))); slug != "" && !seen[slug] {
			seen[slug] = true
			repos = append(repos, slug)
		}
	}

	sort.Strings(repos)

	return repos
}

// githubSlug extracts owner/repo from https and ssh GitHub remotes. Only
// github.com hosts count: a repository on another forge must never be
// checked (and credited as evidence) against a same-named GitHub repo.
func githubSlug(remote string) string {
	remote = strings.TrimSuffix(strings.TrimSpace(remote), ".git")

	var host, path string

	switch {
	case strings.Contains(remote, "://"):
		u, err := url.Parse(remote)
		if err != nil {
			return ""
		}

		host, path = u.Hostname(), u.Path
	case strings.Contains(remote, ":"):
		// scp-like: git@github.com:owner/repo
		at := strings.LastIndex(remote[:strings.Index(remote, ":")], "@")
		host = remote[at+1 : strings.Index(remote, ":")]
		path = remote[strings.Index(remote, ":")+1:]
	default:
		return ""
	}

	if !strings.EqualFold(host, "github.com") && !strings.EqualFold(host, "www.github.com") {
		return ""
	}

	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ""
	}

	return parts[0] + "/" + parts[1]
}
