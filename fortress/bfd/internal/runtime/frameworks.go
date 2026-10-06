package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
)

// DefaultFrameworks are imported on first run. Override with
// BF_DEFAULT_FRAMEWORKS (comma-separated file names without .json).
var DefaultFrameworks = []string{"SOC2", "ISO27001-2022", "GDPR"}

// LibraryDir finds the framework library: BF_LIBRARY_DIR, then
// library/frameworks next to (or one level above) the executable.
func LibraryDir() string {
	if d := os.Getenv("BF_LIBRARY_DIR"); d != "" {
		return d
	}

	exe, err := os.Executable()
	if err != nil {
		return ""
	}

	dir := filepath.Dir(exe)
	for _, c := range []string{
		filepath.Join(dir, "library", "frameworks"),
		filepath.Join(dir, "..", "library", "frameworks"),
	} {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			return filepath.Clean(c)
		}
	}

	return ""
}

func defaultFrameworkNames() []string {
	if v := strings.TrimSpace(os.Getenv("BF_DEFAULT_FRAMEWORKS")); v != "" {
		var names []string
		for _, n := range strings.Split(v, ",") {
			if n = strings.TrimSpace(n); n != "" {
				names = append(names, n)
			}
		}

		return names
	}

	return DefaultFrameworks
}

func (s *Session) frameworkCount(ctx context.Context, orgID string) (int, error) {
	const q = `query($id: ID!) { node(id: $id) { ... on Organization { frameworks(first: 100) { edges { node { id } } } } } }`

	var out struct {
		Node struct {
			Frameworks struct {
				Edges []json.RawMessage `json:"edges"`
			} `json:"frameworks"`
		} `json:"node"`
	}
	if err := s.Do(ctx, "console", q, map[string]any{"id": orgID}, &out); err != nil {
		return 0, err
	}

	return len(out.Node.Frameworks.Edges), nil
}

// ImportFramework uploads a framework file through the GraphQL multipart
// request spec, the same path the console's import dialog uses.
func (s *Session) ImportFramework(ctx context.Context, orgID, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	const q = `mutation($input: ImportFrameworkInput!) { importFramework(input: $input) { frameworkEdge { node { id name } } } }`

	return s.Upload(ctx, q, map[string]any{"organizationId": orgID}, filepath.Base(path), "application/json", data, nil)
}

// Upload runs a console mutation whose input has a single Upload field
// named "file", following the GraphQL multipart request spec.
func (s *Session) Upload(ctx context.Context, query string, input map[string]any, filename, contentType string, data []byte, out any) error {
	vars := map[string]any{}
	for k, v := range input {
		vars[k] = v
	}
	vars["file"] = nil

	ops, err := json.Marshal(map[string]any{"query": query, "variables": map[string]any{"input": vars}})
	if err != nil {
		return err
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("operations", string(ops))
	_ = mw.WriteField("map", `{"0":["variables.input.file"]}`)

	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="0"; filename=%q`, filename))
	h.Set("Content-Type", contentType)

	fw, err := mw.CreatePart(h)
	if err != nil {
		return err
	}

	if _, err := fw.Write(data); err != nil {
		return err
	}

	if err := mw.Close(); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/api/console/v1/graphql", &body)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Errors []gqlError      `json:"errors"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("unexpected response (%d): %s", resp.StatusCode, truncate(raw, 300))
	}

	if len(envelope.Errors) > 0 {
		return &GraphQLError{Errors: envelope.Errors}
	}

	if out != nil && len(envelope.Data) > 0 {
		return json.Unmarshal(envelope.Data, out)
	}

	return nil
}

// importDefaultFrameworks seeds a brand-new organization. It does nothing
// once the organization has any framework, so user deletions stick.
func importDefaultFrameworks(ctx context.Context, sess *Session, orgID string, logf func(string, ...any)) {
	dir := LibraryDir()
	if dir == "" {
		logf("framework library not found; skipping default frameworks")
		return
	}

	if err := sess.AssumeOrganization(ctx, orgID); err != nil {
		logf("%v", err)
		return
	}

	n, err := sess.frameworkCount(ctx, orgID)
	if err != nil {
		logf("cannot list frameworks: %v", err)
		return
	}

	if n > 0 {
		return
	}

	for _, name := range defaultFrameworkNames() {
		path := filepath.Join(dir, name+".json")
		if err := sess.ImportFramework(ctx, orgID, path); err != nil {
			logf("cannot import framework %s: %v", name, err)
			continue
		}

		logf("imported framework %s", name)
	}
}
