// Package validate checks that every generated framework file in
// ../frameworks is importable by Probo, using Probo's own import request
// type, decoding path and field validators.
package validate

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"go.probo.inc/probo/pkg/coredata"
	"go.probo.inc/probo/pkg/probo"
	"go.probo.inc/probo/pkg/validator"
)

const (
	libraryFrameworksDir = "../frameworks"
	proboFrameworksDir   = "../../../probo/apps/console/public/data/frameworks"
)

// decode mirrors pkg/server/api/console/v1/framework_resolvers.go ImportFramework:
// json.NewDecoder(file).Decode(&req.Framework). strict additionally rejects
// fields the importer would silently ignore.
func decode(t *testing.T, path string, strict bool) probo.ImportFrameworkRequest {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	req := probo.ImportFrameworkRequest{}
	dec := json.NewDecoder(bytes.NewReader(data))
	if strict {
		dec.DisallowUnknownFields()
	}
	if err := dec.Decode(&req.Framework); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	if dec.More() {
		t.Fatalf("%s: trailing data after JSON document", path)
	}

	return req
}

func files(t *testing.T, dir string) []string {
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(matches)

	return matches
}

// check applies the constraints Probo enforces on import (DB uniqueness,
// logo handling) plus the validators ControlService applies on create/update,
// so imported controls remain editable in the console.
func check(t *testing.T, path string, req probo.ImportFrameworkRequest, requireDescriptionRules bool) {
	fw := req.Framework

	v := validator.New()
	v.Check(fw.ID, "id", validator.Required(), validator.SafeTextNoNewLine(probo.TitleMaxLength))
	v.Check(fw.Name, "name", validator.Required(), validator.SafeTextNoNewLine(probo.TitleMaxLength))
	if err := v.Error(); err != nil {
		t.Errorf("%s: framework: %v", path, err)
	}

	if fw.Logo != nil && (fw.Logo.Light == "" || fw.Logo.Dark == "") {
		t.Errorf("%s: logo present but light/dark missing (importer uploads both)", path)
	}

	if len(fw.Controls) == 0 {
		t.Errorf("%s: no controls", path)
	}

	seen := map[string]bool{}
	for i, c := range fw.Controls {
		// controls_framework_ref_unique UNIQUE (framework_id, section_title)
		if seen[c.ID] {
			t.Errorf("%s: control[%d] duplicate id %q", path, i, c.ID)
		}
		seen[c.ID] = true

		cv := validator.New()
		cv.Check(c.ID, "id", validator.Required(), validator.SafeTextNoNewLine(probo.TitleMaxLength))
		cv.Check(c.Name, "name", validator.Required(), validator.SafeTextNoNewLine(probo.TitleMaxLength))
		if requireDescriptionRules && c.Description != "" {
			cv.Check(c.Description, "description", validator.SafeText(probo.ContentMaxLength))
		}
		if c.MaturityLevel != nil && !coredata.ControlMaturityLevel(*c.MaturityLevel).IsValid() {
			t.Errorf("%s: control %q invalid maturity_level %q", path, c.ID, *c.MaturityLevel)
		}
		if err := cv.Error(); err != nil {
			t.Errorf("%s: control %q: %v", path, c.ID, err)
		}
	}
}

func TestLibraryFrameworksImportable(t *testing.T) {
	paths := files(t, libraryFrameworksDir)
	if len(paths) == 0 {
		t.Fatal("no generated framework files; run scripts/convert-comp.mjs")
	}

	// frameworks_org_ref_unique UNIQUE (organization_id, reference_id): library
	// IDs must not collide with each other or with Probo's shipped frameworks.
	ids := map[string]string{}
	for _, p := range files(t, proboFrameworksDir) {
		ids[decode(t, p, false).Framework.ID] = p
	}

	for _, p := range paths {
		t.Run(filepath.Base(p), func(t *testing.T) {
			req := decode(t, p, true)
			if other, ok := ids[req.Framework.ID]; ok {
				t.Errorf("framework id %q collides with %s", req.Framework.ID, other)
			}
			ids[req.Framework.ID] = p
			check(t, p, req, true)
			t.Logf("%s: id=%q name=%q controls=%d logo=%v", filepath.Base(p), req.Framework.ID, req.Framework.Name, len(req.Framework.Controls), req.Framework.Logo != nil)
		})
	}
}

// Sanity check of the checker itself: Probo's shipped files must pass the
// same decoding and import constraints.
func TestProboShippedFrameworksPass(t *testing.T) {
	for _, p := range files(t, proboFrameworksDir) {
		t.Run(filepath.Base(p), func(t *testing.T) {
			check(t, p, decode(t, p, true), false)
		})
	}
}
