package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"blackfortress.dev/fortress/bfd/internal/guard"
)

// measureGroup turns a family of guardrail rules into one Probo measure.
// The measure is mapped to every control those rules reference, and each
// day's ledger entries for the family are uploaded to it as evidence.
type measureGroup struct {
	Key         string
	Name        string
	Category    string
	Description string
	Prefixes    []string
}

var measureGroups = []measureGroup{
	{
		Key:      "change",
		Name:     "AI coding agent change management",
		Category: "Change Management",
		Description: "Black Fortress evaluates every file edit and shell command made by AI coding agents " +
			"against change-management rules. Infrastructure, CI/CD, dependency and security-sensitive code " +
			"changes are recorded; destructive or review-bypassing commands require human approval. " +
			"Daily hash-chained ledger extracts are attached as evidence.",
		Prefixes: []string{"change.", "secure-dev."},
	},
	{
		Key:      "secrets",
		Name:     "AI coding agent secret and access safeguards",
		Category: "Access Control",
		Description: "Black Fortress blocks AI coding agents from writing credentials or private keys into files " +
			"and requires human approval to read secret files, pipe remote scripts into a shell, or escalate " +
			"privileges. Daily hash-chained ledger extracts are attached as evidence.",
		Prefixes: []string{"secrets.", "access.", "supply-chain."},
	},
	{
		Key:      "privacy",
		Name:     "AI coding agent privacy review",
		Category: "Privacy",
		Description: "Black Fortress records AI coding agent changes to schemas and migrations that touch personal " +
			"data, and requires approval before personal data is written to logs. Daily hash-chained ledger " +
			"extracts are attached as evidence.",
		Prefixes: []string{"privacy."},
	},
}

func (g measureGroup) owns(ruleID string) bool {
	for _, p := range g.Prefixes {
		if strings.HasPrefix(ruleID, p) {
			return true
		}
	}

	return false
}

type syncState struct {
	Measures     map[string]string      `json:"measures"`
	Linked       map[string][]string    `json:"linked"`
	SyncedDays   []string               `json:"synced_days"`
	CheckUploads map[string]checkUpload `json:"check_uploads,omitempty"`
	// Documents maps Comp policy template ids to Probo document ids.
	Documents        map[string]string `json:"documents,omitempty"`
	PoliciesImported bool              `json:"policies_imported,omitempty"`
}

func loadSyncState(path string) *syncState {
	st := &syncState{Measures: map[string]string{}, Linked: map[string][]string{}}

	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, st)
	}

	if st.Measures == nil {
		st.Measures = map[string]string{}
	}

	if st.Linked == nil {
		st.Linked = map[string][]string{}
	}

	return st
}

func (st *syncState) save(path string) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o600)
}

func (st *syncState) synced(day string) bool {
	for _, d := range st.SyncedDays {
		if d == day {
			return true
		}
	}

	return false
}

type orgControl struct {
	ID           string
	SectionTitle string
	Framework    string
}

func (s *Session) orgControls(ctx context.Context, orgID string) ([]orgControl, error) {
	const q = `query($id: ID!) { node(id: $id) { ... on Organization {
  frameworks(first: 100) { edges { node { name controls(first: 1000) { edges { node { id sectionTitle } } } } } }
} } }`

	var out struct {
		Node struct {
			Frameworks struct {
				Edges []struct {
					Node struct {
						Name     string `json:"name"`
						Controls struct {
							Edges []struct {
								Node struct {
									ID           string `json:"id"`
									SectionTitle string `json:"sectionTitle"`
								} `json:"node"`
							} `json:"edges"`
						} `json:"controls"`
					} `json:"node"`
				} `json:"edges"`
			} `json:"frameworks"`
		} `json:"node"`
	}
	if err := s.Do(ctx, "console", q, map[string]any{"id": orgID}, &out); err != nil {
		return nil, err
	}

	var controls []orgControl

	for _, fe := range out.Node.Frameworks.Edges {
		for _, ce := range fe.Node.Controls.Edges {
			controls = append(controls, orgControl{ID: ce.Node.ID, SectionTitle: ce.Node.SectionTitle, Framework: fe.Node.Name})
		}
	}

	return controls, nil
}

// frameworkKeys maps the prefix of a rule's control reference to a
// fragment of the framework name it belongs to.
var frameworkKeys = map[string]string{
	"SOC2":     "SOC2",
	"ISO27001": "27001",
	"ISO27701": "27701",
	"GDPR":     "GDPR",
	"HIPAA":    "HIPAA",
	"PCIDSS":   "PCI",
	"CCPA":     "CCPA",
}

func compact(s string) string {
	return strings.ToUpper(strings.Join(strings.Fields(s), ""))
}

// matchControls resolves references like "ISO27001:A.8.32" or
// "GDPR:Art.25" to the organization's control ids. A reference also
// matches sub-clauses: "GDPR:Art.25" matches "Art. 25(1)".
func matchControls(refs []string, controls []orgControl) []string {
	seen := map[string]bool{}

	var ids []string

	for _, ref := range refs {
		prefix, code, ok := strings.Cut(ref, ":")
		if !ok {
			continue
		}

		fwKey, ok := frameworkKeys[prefix]
		if !ok {
			continue
		}

		code = compact(code)

		for _, c := range controls {
			if !strings.Contains(compact(c.Framework), fwKey) {
				continue
			}

			title := compact(c.SectionTitle)
			if title == code || strings.HasPrefix(title, code+"(") {
				if !seen[c.ID] {
					seen[c.ID] = true
					ids = append(ids, c.ID)
				}
			}
		}
	}

	sort.Strings(ids)

	return ids
}

// SyncEvidence makes sure each measure group exists and is mapped to its
// controls, then uploads one report per group for each completed day not
// yet synced (and, when includeToday is set, a snapshot of today so far).
func (d *Daemon) SyncEvidence(ctx context.Context, includeToday bool) (int, error) {
	d.syncMu.Lock()
	defer d.syncMu.Unlock()

	sess := NewSession(d.probodURL())
	if err := sess.SignIn(ctx, d.secrets.UserEmail, d.secrets.UserPassword); err != nil {
		return 0, err
	}

	orgID := d.secrets.OrganizationID
	if err := sess.AssumeOrganization(ctx, orgID); err != nil {
		return 0, err
	}

	statePath := filepath.Join(d.layout.Home, "sync.json")
	st := loadSyncState(statePath)

	controls, err := sess.orgControls(ctx, orgID)
	if err != nil {
		return 0, err
	}

	policy, err := guard.LoadPolicy(d.layout.Policy)
	if err != nil {
		if policy, err = guard.DefaultPolicy(); err != nil {
			return 0, err
		}
	}

	for _, g := range measureGroups {
		if err := sess.ensureMeasure(ctx, orgID, g, st); err != nil {
			return 0, fmt.Errorf("measure %q: %w", g.Name, err)
		}

		var refs []string
		for _, r := range policy.Rules {
			if !r.Disabled && g.owns(r.ID) {
				refs = append(refs, r.Controls...)
			}
		}

		if err := sess.linkControls(ctx, st, g.Key, matchControls(refs, controls)); err != nil {
			return 0, fmt.Errorf("measure %q: %w", g.Name, err)
		}
	}

	if err := st.save(statePath); err != nil {
		return 0, err
	}

	days, err := ledgerDays(d.layout.Ledger)
	if err != nil {
		return 0, err
	}

	today := time.Now().UTC().Format("2006-01-02")
	uploaded := 0

	for _, day := range days {
		isToday := day == today
		if (isToday && !includeToday) || (!isToday && st.synced(day)) {
			continue
		}

		n, err := d.uploadDay(ctx, sess, st, day, isToday)
		if err != nil {
			return uploaded, err
		}

		uploaded += n

		if !isToday {
			st.SyncedDays = append(st.SyncedDays, day)
			if err := st.save(statePath); err != nil {
				return uploaded, err
			}
		}
	}

	if uploaded > 0 {
		d.kickPosture()
	}

	return uploaded, nil
}

func (s *Session) ensureMeasure(ctx context.Context, orgID string, g measureGroup, st *syncState) error {
	if id := st.Measures[g.Key]; id != "" {
		var out struct {
			Node *struct {
				ID string `json:"id"`
			} `json:"node"`
		}
		if err := s.Do(ctx, "console", `query($id: ID!) { node(id: $id) { id } }`, map[string]any{"id": id}, &out); err == nil && out.Node != nil {
			return nil
		}

		// The measure was deleted in the console; recreate it and its links.
		delete(st.Measures, g.Key)
		delete(st.Linked, g.Key)
	}

	const q = `mutation($input: CreateMeasureInput!) { createMeasure(input: $input) { measureEdge { node { id } } } }`

	var out struct {
		CreateMeasure struct {
			MeasureEdge struct {
				Node struct {
					ID string `json:"id"`
				} `json:"node"`
			} `json:"measureEdge"`
		} `json:"createMeasure"`
	}
	if err := s.Do(ctx, "console", q, map[string]any{"input": map[string]any{
		"organizationId": orgID,
		"name":           g.Name,
		"description":    g.Description,
		"category":       g.Category,
	}}, &out); err != nil {
		return err
	}

	id := out.CreateMeasure.MeasureEdge.Node.ID
	if id == "" {
		return errors.New("empty measure id")
	}

	st.Measures[g.Key] = id

	// Guardrails are enforced from the moment they are installed, but
	// whether that fully implements the measure is the owner's call.
	const upd = `mutation($input: UpdateMeasureInput!) { updateMeasure(input: $input) { measure { id } } }`

	return s.Do(ctx, "console", upd, map[string]any{"input": map[string]any{"id": id, "state": "IN_PROGRESS"}}, nil)
}

func (s *Session) linkControls(ctx context.Context, st *syncState, key string, controlIDs []string) error {
	linked := map[string]bool{}
	for _, id := range st.Linked[key] {
		linked[id] = true
	}

	const q = `mutation($input: CreateControlMeasureMappingInput!) { createControlMeasureMapping(input: $input) { measureEdge { node { id } } } }`

	for _, cid := range controlIDs {
		if linked[cid] {
			continue
		}

		err := s.Do(ctx, "console", q, map[string]any{"input": map[string]any{"controlId": cid, "measureId": st.Measures[key]}}, nil)
		if err != nil && !strings.Contains(strings.ToLower(err.Error()), "already") {
			return err
		}

		st.Linked[key] = append(st.Linked[key], cid)
	}

	return nil
}

func ledgerDays(dir string) ([]string, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		return nil, err
	}

	days := make([]string, 0, len(files))
	for _, f := range files {
		days = append(days, strings.TrimSuffix(filepath.Base(f), ".jsonl"))
	}

	sort.Strings(days)

	return days, nil
}

func (d *Daemon) uploadDay(ctx context.Context, sess *Session, st *syncState, day string, partial bool) (int, error) {
	entries, err := guard.ReadDay(d.layout.Ledger, day)
	if err != nil {
		return 0, err
	}

	verified, verifyErr := guard.VerifyDay(d.layout.Ledger, day)

	uploaded := 0

	for _, g := range measureGroups {
		var mine []guard.Entry
		for _, e := range entries {
			for _, r := range e.Rules {
				if g.owns(r) {
					mine = append(mine, e)
					break
				}
			}
		}

		if len(mine) == 0 {
			continue
		}

		report := renderReport(g, day, partial, mine, entries, verified, verifyErr)

		name := fmt.Sprintf("black-fortress-%s-%s.md", g.Key, day)
		if partial {
			name = fmt.Sprintf("black-fortress-%s-%s-partial-%s.md", g.Key, day, time.Now().UTC().Format("150405"))
		}

		const q = `mutation($input: UploadMeasureEvidenceInput!) { uploadMeasureEvidence(input: $input) { evidenceEdge { node { id } } } }`
		if err := sess.Upload(ctx, q, map[string]any{"measureId": st.Measures[g.Key]}, name, "text/markdown", []byte(report), nil); err != nil {
			return uploaded, fmt.Errorf("upload %s: %w", name, err)
		}

		uploaded++
	}

	return uploaded, nil
}

func renderReport(g measureGroup, day string, partial bool, mine, all []guard.Entry, verified int, verifyErr error) string {
	var b strings.Builder

	title := "Black Fortress agent evidence: " + g.Name
	fmt.Fprintf(&b, "# %s\n\n", title)
	fmt.Fprintf(&b, "- **Day (UTC):** %s%s\n", day, map[bool]string{true: " (partial snapshot)", false: ""}[partial])
	fmt.Fprintf(&b, "- **Generated:** %s\n", time.Now().UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "- **Entries in this report:** %d of %d ledger entries for the day\n", len(mine), len(all))

	if verifyErr != nil {
		fmt.Fprintf(&b, "- **Ledger integrity:** FAILED — %v\n", verifyErr)
	} else {
		fmt.Fprintf(&b, "- **Ledger integrity:** verified, %d entries hash-chained\n", verified)
	}

	if n := len(all); n > 0 {
		fmt.Fprintf(&b, "- **Chain head (SHA-256):** `%s`\n", all[n-1].Hash)
	}

	counts := map[string]int{}
	for _, e := range mine {
		counts[e.Outcome]++
	}

	b.WriteString("\n## Outcomes\n\n| Outcome | Count |\n|---|---|\n")

	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	for _, k := range keys {
		label := k
		if label == "" {
			label = "(none)"
		}

		fmt.Fprintf(&b, "| %s | %d |\n", label, counts[k])
	}

	b.WriteString("\n## Entries\n\n| Time (UTC) | Agent | Event | Tool | Target | Decision | Outcome | Rules | Controls |\n|---|---|---|---|---|---|---|---|---|\n")

	for _, e := range mine {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s | %s | %s |\n",
			e.Time.UTC().Format("15:04:05"), cell(e.Agent), cell(e.Event), cell(e.Tool), cell(e.Target),
			cell(string(e.Decision)), cell(e.Outcome), cell(strings.Join(e.Rules, ", ")), cell(strings.Join(e.Controls, ", ")))
	}

	b.WriteString("\nEach entry carries the SHA-256 of its predecessor; `bf ledger verify` on the originating machine re-checks the full chain.\n")

	return b.String()
}

func cell(s string) string {
	s = strings.ReplaceAll(s, "|", `\|`)
	s = strings.ReplaceAll(s, "\n", " ")

	if len(s) > 120 {
		s = s[:120] + "…"
	}

	return s
}
