# Black Fortress control library

Comp AI's framework library (frameworks, requirements, control/policy/task
templates), converted to files Probo can import without changes. It also
includes a crosswalk to the frameworks Probo already ships.

```
library/
  scripts/convert-comp.mjs   converter (Node ESM, no dependencies)
  frameworks/*.json          Probo-importable framework files (one per visible Comp framework)
  crosswalk.json             Probo control ids -> Comp requirement identifiers
  processes/controls.json    Comp control templates + requirement/policy/task links
  processes/policies.json    Comp policy templates (full TipTap content) + links
  processes/tasks.json       Comp task templates (evidence tasks) + links
  manifest.json              counts, skipped frameworks, match rates (machine-readable)
  validate/                  Go module that checks frameworks/*.json against Probo's importer
```

## Regenerate

From the repository root:

```sh
node fortress/library/scripts/convert-comp.mjs     # writes frameworks/, processes/, crosswalk.json, manifest.json
(cd fortress/library/validate && go test ./...)     # every framework file must pass
```

Inputs:
- `packages/db/prisma/seed/primitives/FrameworkEditor{Framework,Requirement,ControlTemplate,PolicyTemplate,TaskTemplate}.json`
- `packages/db/prisma/seed/relations/_FrameworkEditorControlTemplateToFrameworkEditor{Requirement,PolicyTemplate,TaskTemplate}.json`
  (implicit Prisma many-to-many tables: `A` is the control template id, `B` is the other side)
- `probo/apps/console/public/data/frameworks/*.json` (used for the crosswalk and to reuse logos)

Optional flags are `--comp-root DIR` and `--probo-root DIR`. The output is
deterministic: it has no timestamps and is sorted, so diffs stay small.

## Framework file format (Probo import)

This is the format that `probo.ImportFrameworkRequest` decodes in
`probo/pkg/probo/framework_service.go`. The `importFramework` mutation uploads
the file, and `framework_resolvers.go` decodes it with `json.NewDecoder(file).Decode(&req.Framework)`:

```json
{
  "id": "COMP-SOC2",                 // framework reference_id, unique per organization
  "name": "SOC 2 (Comp)",
  "logo": { "light": "<svg…>", "dark": "<svg…>" },   // optional; both are uploaded as SVG
  "controls": [
    { "id": "CC6.1", "name": "…", "description": "…" }  // id -> control section_title, unique per framework
  ]
}
```

Controls can also carry `best_practice` (bool, default true), `maturity_level`
(`NONE|INITIAL|MANAGED|DEFINED|OPTIMIZING`, default `INITIAL`) and
`not_implemented_justification`. The generated files don't use them.

To import a file, upload it in the console under Frameworks → Import → file.
The console's quick-pick menu is a hard-coded list in
`probo/packages/ui/src/Molecules/FrameworkSelector/FrameworkSelector.tsx` that
loads `/data/frameworks/<id>.json`. To add these files to that menu, copy them
into `probo/apps/console/public/data/frameworks/` and add entries to the list.

## Frameworks

| File | Probo id | Controls | Comp source (version) | Probo equivalent |
|---|---|---:|---|---|
| `comp-soc2.json` | COMP-SOC2 | 60 | SOC 2 (1); 63 requirements, 4× P3.2 merged | SOC2 |
| `comp-iso27001.json` | COMP-ISO27001 | 119 | ISO 27001 (2022) | ISO27001-2022 |
| `comp-hipaa.json` | COMP-HIPAA | 77 | HIPAA (2025) | HIPAA |
| `comp-gdpr.json` | COMP-GDPR | 19 | GDPR (1.0.0) | GDPR |
| `comp-iso42001.json` | COMP-ISO42001 | 70 | ISO 42001 (1.0.0) | ISO42001-2023 |
| `comp-pci-dss.json` | COMP-PCI-DSS | 233 | PCI DSS Level 1 (1.0.0) | PCI-DSS |
| `comp-ccpa.json` | COMP-CCPA | 12 | CCPA (1.0.0) | CCPA |
| `iso9001.json` | ISO9001-2015 | 37 | ISO 9001 (1.0.0) | — (new to Probo) |

A framework that Probo already ships gets a `comp-` prefix and a `COMP-` id,
so both versions can be imported into the same organization. The `comp-*`
files reuse the logo from Probo's version of the same framework. The ISO 9001
file has no logo.

Normalization applied by the converter:
- Identifiers are trimmed. Comp's ISO 27001 data contains stray tabs and spaces.
- ISO 42001 identifiers lose the `ISO/IEC 42001:2023 Clause ` prefix, so `4.1` and `A.2.2` match Probo's.
- HIPAA identifiers are rewritten from `164.308(a.1.ii.A)` to `164.308(a)(1)(ii)(A)`.
- Comp's `164.310(d)(1)(i–iv)` is corrected to `164.310(d)(2)(i–iv)`, the regulation's
  numbering for Disposal, Media re-use, Accountability and Data backup.
- Control names follow Probo's style. ISO 27001 uses `Category - Title`, and HIPAA uses
  `Safeguard - Heading`. A leading copy of the identifier is stripped.
- Mojibake is fixed (`‚Äô` and the corrupted `entity‚Entity` become `entity’s`), and
  surrounding quotes are stripped.
- Requirements with the same identifier are merged into one control. Their
  descriptions become a bulleted list, and every source requirement id is kept
  in the crosswalk.
- In PCI DSS Level 1, Comp's description holds the evidence list. It is emitted
  as `Evidence: …`.

### Skipped Comp frameworks (15)

| Comp framework | Reason |
|---|---|
| NIS 2, NIST CSF, SOC 2 (old, `frk_681ebae2…`) | hidden; requirements have no identifiers |
| SOC 2 v.1, PCI v0 | hidden; superseded draft; no identifiers |
| SOC 2 Type 1, SOC 2 v2 | hidden; draft/superseded |
| Example PCI, GDPR (Test) | hidden; test/example |
| Cloud Cybersecurity Controls, NIST 800-53, PCI Level 1 | hidden; no requirements |
| PIPEDA | hidden (draft, 10 requirements) |
| PCI DSS (`frk_68c1ead2…`) | visible, but a placeholder with one generic requirement. Superseded by PCI DSS Level 1 |
| NEN 7510 | visible, but a placeholder with one generic requirement |

`manifest.json` lists the exact reason for each one.

## Crosswalk (`crosswalk.json`)

The crosswalk maps each Probo control to Comp requirements:
`{ probo, proboName, matchType, comp: [{ identifier, rawIdentifiers, requirementIds }] }`.

`matchType` is one of:
- `exact`: the normalized reference codes are equal (`CC6.1`, `A.5.1`, `164.308(a)(1)(i)`).
- `ancestor`: Comp is coarser, and the Probo control maps to its nearest Comp parent (Probo `9.2.1` → Comp `9.2`).
- `descendant`: Comp is finer, and the Probo control maps to all of its Comp children (Probo `1.1` → Comp `1.1.1`, `1.1.2`).
- `curated`: a hand-written map in the script, used only where the codes cannot match as text.
  GDPR and CCPA use this because Comp uses its own codes (`DS-5`, `CCPA-9`). HIPAA uses it for one
  entry: Comp `164.308(b)(3)` (post-2013 numbering) maps to Probo `164.308(b)(4)`.
- `unmatched`

| Probo framework | Probo controls matched | Comp requirements matched | exact / ancestor / descendant / curated |
|---|---:|---:|---|
| SOC2 | 60/61 (98.4%) | 60/60 (100%) | 60 / 0 / 0 / 0 |
| ISO27001-2022 | 121/123 (98.4%) | 119/119 (100%) | 115 / 5 / 1 / 0 |
| HIPAA | 60/60 (100%) | 59/77 (76.6%) | 57 / 2 / 0 / 1 |
| GDPR | 39/62 (62.9%) | 19/19 (100%) | 0 / 0 / 0 / 39 |
| ISO42001-2023 | 70/70 (100%) | 70/70 (100%) | 70 / 0 / 0 / 0 |
| PCI-DSS | 302/331 (91.2%) | 233/233 (100%) | 231 / 8 / 63 / 0 |
| CCPA | 19/23 (82.6%) | 9/12 (75.0%) | 0 / 0 / 0 / 19 |

These gaps are expected:
- SOC2 `C1.2` is missing from Comp.
- ISO 27001 `6.2` and `6.3` are missing from Comp.
- PCI DSS appendices A1–A3 and 11 v4.0.1 sub-requirements are missing from Comp.
- Comp's HIPAA also covers §164.306 and §164.314, which Probo doesn't.
- The Comp GDPR and CCPA frameworks are short checklists, so many articles and sections have no counterpart.

## Processes (`processes/`)

These are the templates later phases will seed from:

| File | Records | Linked | Content |
|---|---:|---:|---|
| `controls.json` | 204 control templates | 188 have requirements; 157 map into a framework in `frameworks/` | name, description, documentTypes, `requirements[]`, `policyTemplateIds[]`, `taskTemplateIds[]` |
| `policies.json` | 52 policy templates | 51 linked to controls | name, description, frequency, department, `content` (TipTap/ProseMirror JSON, normalized to a `doc` node), `controlTemplateIds[]`, `requirementRefs` |
| `tasks.json` | 148 task templates | 135 linked to controls | name, description, frequency, department, automationStatus, `controlTemplateIds[]`, `requirementRefs` |

Each entry in `controls[].requirements[]` keeps:
- the Comp requirement id, framework and raw identifier;
- `library`: the `{framework, control}` it became in `frameworks/`, or `null` if its framework was skipped;
- `probo`: the native Probo `{framework, control, matchType}` refs from the crosswalk.

For policies and tasks, `requirementRefs` is the union of those refs across the
linked controls. Control links that point to skipped (hidden) frameworks are
kept, labeled `(hidden)`.

Source link counts are 1453 control↔requirement, 193 control↔policy and 314
control↔task. No links are dangling.

## Validation (`validate/`)

`validate/` is a small Go module. It points at the local Probo checkout with
`replace go.probo.inc/probo => ../../../probo`. Its tests decode every file in
`frameworks/` into `probo.ImportFrameworkRequest` the same way the GraphQL
resolver does, and reject unknown fields. They then check that:
- framework ids are unique across the library and Probo's shipped files (`frameworks_org_ref_unique`);
- control ids are unique within each framework (`controls_framework_ref_unique`);
- each logo has both a light and a dark variant;
- `maturity_level`, if set, is valid;
- each control's id, name and description pass the same `validator.SafeTextNoNewLine` /
  `SafeText` rules as `ControlService`, so imported controls stay editable.

The same checks also run against Probo's 13 shipped files, to test the checker itself.

## License

- Content from Comp AI is **AGPL-3.0** (Copyright Comp AI, Inc.). This covers every
  file in `frameworks/`, `processes/` and the Comp side of `crosswalk.json`.
  Redistributing it, or serving it over a network as part of a modified
  program, carries AGPL obligations.
- Content from Probo is **MIT** (Copyright Probo Inc.). This covers the framework
  logos copied into `comp-*.json` and the Probo control ids and names in `crosswalk.json`.
- The converter and the validator are part of Black Fortress. Because they are
  derived from and distributed with the AGPL content, treat the whole
  `fortress/library/` directory as AGPL-3.0.
