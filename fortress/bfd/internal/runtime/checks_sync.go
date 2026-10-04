package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// libraryIndex resolves a Comp task template (a check's taskMapping) to the
// Probo framework controls it satisfies, using fortress/library.
type libraryIndex struct {
	// task template id -> refs of (framework name, control section)
	taskRefs map[string][]controlRef
}

type controlRef struct {
	Framework string
	Control   string
}

func loadLibraryIndex() (*libraryIndex, error) {
	dir := LibraryDir()
	if dir == "" {
		return nil, fmt.Errorf("framework library not found")
	}

	// Framework files are referenced by their import id; organizations see
	// them by name.
	names := map[string]string{}

	files, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	for _, f := range files {
		var fw struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}

		data, err := os.ReadFile(f)
		if err == nil && json.Unmarshal(data, &fw) == nil && fw.ID != "" {
			names[strings.TrimSuffix(filepath.Base(f), ".json")] = fw.Name
			names[fw.ID] = fw.Name
		}
	}

	data, err := os.ReadFile(filepath.Join(filepath.Dir(dir), "processes", "tasks.json"))
	if err != nil {
		return nil, fmt.Errorf("task library: %w", err)
	}

	var lib struct {
		Tasks []struct {
			ID              string `json:"id"`
			RequirementRefs struct {
				Probo []struct {
					Framework string `json:"framework"`
					Control   string `json:"control"`
				} `json:"probo"`
			} `json:"requirementRefs"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(data, &lib); err != nil {
		return nil, fmt.Errorf("task library: %w", err)
	}

	idx := &libraryIndex{taskRefs: map[string][]controlRef{}}

	for _, t := range lib.Tasks {
		for _, r := range t.RequirementRefs.Probo {
			if name := names[r.Framework]; name != "" {
				idx.taskRefs[t.ID] = append(idx.taskRefs[t.ID], controlRef{Framework: name, Control: r.Control})
			}
		}
	}

	return idx, nil
}

func (idx *libraryIndex) controlsFor(taskID string, controls []orgControl) []string {
	seen := map[string]bool{}

	var ids []string

	for _, ref := range idx.taskRefs[taskID] {
		code := compact(ref.Control)

		for _, c := range controls {
			if compact(c.Framework) != compact(ref.Framework) {
				continue
			}

			title := compact(c.SectionTitle)
			if (title == code || strings.HasPrefix(title, code+"(")) && !seen[c.ID] {
				seen[c.ID] = true
				ids = append(ids, c.ID)
			}
		}
	}

	sort.Strings(ids)

	return ids
}

type checkUpload struct {
	Day         string `json:"day"`
	Fingerprint string `json:"fingerprint"`
}

// syncCheckEvidence keeps one Probo measure per automated check: mapped to
// the controls of the check's task template, its state following the latest
// result, and a report uploaded whenever the result changes (at most daily
// when unchanged).
func (d *Daemon) syncCheckEvidence(ctx context.Context, st *ChecksState) error {
	idx, err := loadLibraryIndex()
	if err != nil {
		return err
	}

	d.syncMu.Lock()
	defer d.syncMu.Unlock()

	sess := NewSession(d.probodURL())
	if err := sess.SignIn(ctx, d.secrets.UserEmail, d.secrets.UserPassword); err != nil {
		return err
	}

	orgID := d.secrets.OrganizationID
	if err := sess.AssumeOrganization(ctx, orgID); err != nil {
		return err
	}

	controls, err := sess.orgControls(ctx, orgID)
	if err != nil {
		return err
	}

	statePath := filepath.Join(d.layout.Home, "sync.json")
	sync := loadSyncState(statePath)
	defer func() { _ = sync.save(statePath) }()

	if sync.CheckUploads == nil {
		sync.CheckUploads = map[string]checkUpload{}
	}

	today := time.Now().UTC().Format("2006-01-02")

	for _, run := range st.Providers {
		for _, c := range run.Checks {
			verdict := c.Verdict()
			if verdict == "error" {
				continue
			}

			key := "check:" + run.Provider + "/" + c.ID
			group := measureGroup{
				Key:         key,
				Name:        fmt.Sprintf("Automated check: %s — %s", run.ProviderName, c.Name),
				Category:    "Automated Checks",
				Description: c.Description + " Verified automatically by Black Fortress from the developer's " + run.ProviderName + " access.",
			}

			if err := sess.ensureMeasure(ctx, orgID, group, sync); err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}

			if err := sess.linkControls(ctx, sync, key, idx.controlsFor(c.TaskMapping, controls)); err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}

			// Only passing evidence implements a measure; a check that found
			// nothing to evaluate leaves it in progress.
			state := map[string]string{"pass": "IMPLEMENTED", "fail": "NOT_IMPLEMENTED"}[verdict]
			if state == "" {
				state = "IN_PROGRESS"
			}

			const upd = `mutation($input: UpdateMeasureInput!) { updateMeasure(input: $input) { measure { id } } }`
			if err := sess.Do(ctx, "console", upd, map[string]any{"input": map[string]any{"id": sync.Measures[key], "state": state}}, nil); err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}

			fp := fingerprint(c)
			if prev := sync.CheckUploads[key]; prev.Fingerprint == fp && prev.Day == today {
				continue
			}

			name := fmt.Sprintf("black-fortress-%s-%s-%s.md", run.Provider, c.ID, time.Now().UTC().Format("20060102-150405"))

			const q = `mutation($input: UploadMeasureEvidenceInput!) { uploadMeasureEvidence(input: $input) { evidenceEdge { node { id } } } }`
			if err := sess.Upload(ctx, q, map[string]any{"measureId": sync.Measures[key]}, name, "text/markdown", []byte(renderCheckReport(run, c)), nil); err != nil {
				return fmt.Errorf("%s: upload: %w", key, err)
			}

			sync.CheckUploads[key] = checkUpload{Day: today, Fingerprint: fp}
		}
	}

	return nil
}

// fingerprint identifies a result by status and the set of resources that
// passed or failed, ignoring timestamps inside evidence.
func fingerprint(c CheckReport) string {
	var parts []string

	parts = append(parts, c.Status)
	for _, o := range c.Passed {
		parts = append(parts, "P|"+o.ResourceType+"|"+o.ResourceID+"|"+o.Title)
	}

	for _, o := range c.Findings {
		parts = append(parts, "F|"+o.ResourceType+"|"+o.ResourceID+"|"+o.Title)
	}

	sort.Strings(parts[1:])
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))

	return hex.EncodeToString(sum[:])
}

func renderCheckReport(run *ProviderRun, c CheckReport) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# Automated check: %s — %s\n\n", run.ProviderName, c.Name)
	fmt.Fprintf(&b, "%s\n\n", c.Description)
	fmt.Fprintf(&b, "- **Result:** %s (%d passing, %d findings)\n", strings.ToUpper(c.Verdict()), len(c.Passed), len(c.Findings))
	fmt.Fprintf(&b, "- **Run at (UTC):** %s\n", run.RanAt.UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "- **Credential source:** %s\n", run.Source)
	fmt.Fprintf(&b, "- **Check id:** `%s/%s`\n", run.Provider, c.ID)

	if len(c.Findings) > 0 {
		b.WriteString("\n## Findings\n\n| Severity | Resource | Issue | Remediation |\n|---|---|---|---|\n")

		for _, f := range c.Findings {
			fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", cell(f.Severity), cell(f.ResourceType+" "+f.ResourceID), cell(f.Title+": "+f.Description), cell(f.Remediation))
		}
	}

	if len(c.Passed) > 0 {
		b.WriteString("\n## Passing\n\n| Resource | Result |\n|---|---|\n")

		for _, p := range c.Passed {
			fmt.Fprintf(&b, "| %s | %s |\n", cell(p.ResourceType+" "+p.ResourceID), cell(p.Title))
		}
	}

	b.WriteString("\n## Raw evidence\n\n```json\n")

	raw, _ := json.MarshalIndent(map[string]any{"passed": c.Passed, "findings": c.Findings}, "", "  ")
	if len(raw) > 64<<10 {
		raw = append(raw[:64<<10], []byte("\n… truncated")...)
	}

	b.Write(raw)
	b.WriteString("\n```\n")

	return b.String()
}
