package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type libraryPolicy struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Content         any    `json:"content"`
	RequirementRefs struct {
		Probo []struct {
			Framework string `json:"framework"`
			Control   string `json:"control"`
		} `json:"probo"`
	} `json:"requirementRefs"`
}

// companyProfile fills policy placeholders. Black Fortress has no onboarding
// questionnaire, so values come from $BF_HOME/company.json (keys COMPANY,
// COMPANYINFO, INDUSTRY, EMPLOYEES, DEVICES, SOFTWARE, LOCATION, CRITICAL,
// DATA, GEO); unset ones render as "[to be defined: …]" for the owner to fill.
func (d *Daemon) companyProfile() map[string]string {
	vars := map[string]string{"COMPANY": localOrgName()}

	data, err := os.ReadFile(filepath.Join(d.layout.Home, "company.json"))
	if err != nil {
		return vars
	}

	var user map[string]string
	if json.Unmarshal(data, &user) == nil {
		for k, v := range user {
			vars[strings.ToUpper(k)] = v
		}
	}

	return vars
}

// importPolicies creates Comp's policy templates as Probo policy documents
// once, rendered for the organization's frameworks and linked to the
// controls each policy covers. Set BF_IMPORT_POLICIES=0 to skip.
func (d *Daemon) importPolicies(ctx context.Context, sess *Session) error {
	if os.Getenv("BF_IMPORT_POLICIES") == "0" {
		return nil
	}

	statePath := filepath.Join(d.layout.Home, "sync.json")
	st := loadSyncState(statePath)

	if st.PoliciesImported {
		return nil
	}

	dir := LibraryDir()
	if dir == "" {
		return fmt.Errorf("framework library not found")
	}

	data, err := os.ReadFile(filepath.Join(filepath.Dir(dir), "processes", "policies.json"))
	if err != nil {
		return fmt.Errorf("policy library: %w", err)
	}

	var lib struct {
		Policies []libraryPolicy `json:"policies"`
	}
	if err := json.Unmarshal(data, &lib); err != nil {
		return fmt.Errorf("policy library: %w", err)
	}

	idx, err := loadLibraryIndex()
	if err != nil {
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

	frameworkNames := map[string]bool{}
	for _, c := range controls {
		frameworkNames[c.Framework] = true
	}

	names := make([]string, 0, len(frameworkNames))
	for n := range frameworkNames {
		names = append(names, n)
	}

	flags := frameworkTemplateFlags(names)
	vars := d.companyProfile()

	if st.Documents == nil {
		st.Documents = map[string]string{}
	}

	sort.Slice(lib.Policies, func(i, j int) bool { return lib.Policies[i].Name < lib.Policies[j].Name })

	for _, p := range lib.Policies {
		if st.Documents[p.ID] != "" {
			continue
		}

		var refs []controlRef
		for _, r := range p.RequirementRefs.Probo {
			if name := idx.frameworkNames[r.Framework]; name != "" {
				refs = append(refs, controlRef{Framework: name, Control: r.Control})
			}
		}

		controlIDs := matchRefs(refs, controls)

		// Policies for frameworks the organization hasn't adopted would only
		// add noise; they can be imported later from the console.
		if len(controlIDs) == 0 {
			continue
		}

		content, err := json.Marshal(renderPolicy(p.Content, vars, flags))
		if err != nil {
			return err
		}

		docID, err := sess.createPolicyDocument(ctx, orgID, p.Name, string(content))
		if err != nil {
			return fmt.Errorf("policy %q: %w", p.Name, err)
		}

		st.Documents[p.ID] = docID

		const q = `mutation($input: CreateControlDocumentMappingInput!) { createControlDocumentMapping(input: $input) { documentEdge { node { id } } } }`
		for _, cid := range controlIDs {
			err := sess.Do(ctx, "console", q, map[string]any{"input": map[string]any{"controlId": cid, "documentId": docID}}, nil)
			if err != nil && !strings.Contains(strings.ToLower(err.Error()), "already") {
				return fmt.Errorf("policy %q: %w", p.Name, err)
			}
		}

		if err := st.save(statePath); err != nil {
			return err
		}
	}

	st.PoliciesImported = true
	d.logger.Printf("imported %d policy documents", len(st.Documents))

	return st.save(statePath)
}

func (s *Session) createPolicyDocument(ctx context.Context, orgID, title, content string) (string, error) {
	const q = `mutation($input: CreateDocumentInput!) { createDocument(input: $input) { documentEdge { node { id } } } }`

	var out struct {
		CreateDocument struct {
			DocumentEdge struct {
				Node struct {
					ID string `json:"id"`
				} `json:"node"`
			} `json:"documentEdge"`
		} `json:"createDocument"`
	}

	err := s.Do(ctx, "console", q, map[string]any{"input": map[string]any{
		"organizationId": orgID,
		"title":          title,
		"content":        content,
		"documentType":   "POLICY",
		"classification": "INTERNAL",
	}}, &out)
	if err != nil {
		return "", err
	}

	return out.CreateDocument.DocumentEdge.Node.ID, nil
}
