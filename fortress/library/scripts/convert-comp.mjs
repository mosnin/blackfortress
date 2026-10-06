#!/usr/bin/env node
// Converts Comp AI's framework-editor seed data into Probo-importable
// framework JSON files, a Probo<->Comp crosswalk, and clean "process"
// (control / policy / task template) files.
//
// Usage: node fortress/library/scripts/convert-comp.mjs [--comp-root DIR] [--probo-root DIR]
// No external dependencies. Output is deterministic (sorted, no timestamps).

import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const libDir = path.resolve(here, "..");
const args = process.argv.slice(2);
const argVal = (flag, def) => {
  const i = args.indexOf(flag);
  return i >= 0 ? args[i + 1] : def;
};
const compRoot = path.resolve(argVal("--comp-root", path.resolve(libDir, "../..")));
const proboRoot = path.resolve(argVal("--probo-root", path.resolve(compRoot, "probo")));

const seedDir = path.join(compRoot, "packages/db/prisma/seed");
const proboFrameworksDir = path.join(proboRoot, "apps/console/public/data/frameworks");
const outFrameworks = path.join(libDir, "frameworks");
const outProcesses = path.join(libDir, "processes");

const readJSON = (p) => JSON.parse(fs.readFileSync(p, "utf8"));
const writeJSON = (p, data) => {
  fs.mkdirSync(path.dirname(p), { recursive: true });
  fs.writeFileSync(p, JSON.stringify(data, null, 2) + "\n");
};

// ---------------------------------------------------------------------------
// Load Comp seed data
// ---------------------------------------------------------------------------

const prim = (n) => readJSON(path.join(seedDir, "primitives", `FrameworkEditor${n}.json`));
const rel = (n) =>
  readJSON(path.join(seedDir, "relations", `_FrameworkEditorControlTemplateToFrameworkEditor${n}.json`));

const compFrameworks = prim("Framework");
const compRequirements = prim("Requirement");
const compControls = prim("ControlTemplate");
const compPolicies = prim("PolicyTemplate");
const compTasks = prim("TaskTemplate");
// Implicit Prisma m2m tables: A = ControlTemplate id, B = other side id.
const ctToReq = rel("Requirement");
const ctToPolicy = rel("PolicyTemplate");
const ctToTask = rel("TaskTemplate");

// ---------------------------------------------------------------------------
// Text cleanup
// ---------------------------------------------------------------------------

function cleanText(s) {
  if (s == null) return "";
  return String(s)
    .replace(/\r\n?/g, "\n")
    .replace(/‚Äô/g, "’")
    .replace(/‚Äú/g, "“")
    .replace(/‚Äù/g, "”")
    .replace(/‚Äì/g, "–")
    .replace(/‚Äî/g, "—")
    // Corrupted "entity’s" in Comp's SOC 2 privacy criteria ("entity‚Entity objectives", "entity‚objectives", ...)
    .replace(/entity‚\s?(?:[Ee]ntity\s?)?/g, "entity’s ")
    .replace(/[ \t]+\n/g, "\n")
    .replace(/\n{3,}/g, "\n\n")
    .trim()
    // Some Comp texts (ISO 42001) are wrapped in literal double quotes.
    .replace(/^"([^"]*)"$/s, "$1");
}
const oneLine = (s) => cleanText(s).replace(/\s+/g, " ").trim();

// ---------------------------------------------------------------------------
// Reference-code normalization
// ---------------------------------------------------------------------------

const trimId = (id) => String(id ?? "").replace(/\s+/g, " ").trim();

// "164.308(a.1.ii.A)" -> "164.308(a)(1)(ii)(A)"
// Erratum: Comp labels the 164.310(d) implementation specifications (Disposal,
// Media re-use, Accountability, Data backup) as (d)(1)(i-iv); the regulation
// numbers them (d)(2)(i-iv). The raw Comp identifier is kept in the crosswalk.
function hipaaId(raw) {
  const id = trimId(raw);
  const m = id.match(/^(\d+\.\d+)\(([^)]*)\)$/);
  if (!m) return id;
  const parts = m[2].split(".").filter(Boolean);
  if (m[1] === "164.310" && parts.length === 3 && parts[0] === "d" && parts[1] === "1") parts[1] = "2";
  return (
    m[1] +
    parts
      .map((p, i) => `(${i === 3 && /^[a-z]$/i.test(p) ? p.toUpperCase() : p})`)
      .join("")
  );
}

const iso42001Id = (raw) =>
  trimId(raw)
    .replace(/^ISO\/IEC\s*42001:2023\s*/i, "")
    .replace(/^Clause\s*/i, "");

// Matching key: lowercase, no spaces, no "Art."/"Clause"/"§" prefixes.
function refKey(id) {
  return String(id)
    .toLowerCase()
    .replace(/\s+/g, "")
    .replace(/^(art\.?|article|clause|§)/, "")
    .replace(/\.$/, "");
}
const refTokens = (id) => refKey(id).split(/[.()]+/).filter(Boolean);

// ---------------------------------------------------------------------------
// Per-framework configuration
// ---------------------------------------------------------------------------

// Keyed by Comp framework id. Only visible, non-placeholder frameworks are emitted.
const FRAMEWORKS = {
  frk_683f377429b8408d1c85f9bd: {
    slug: "soc2",
    id: "COMP-SOC2",
    name: "SOC 2 (Comp)",
    probo: "SOC2",
    normalize: (r) => trimId(r).toUpperCase(),
    title: "plain",
  },
  frk_681ecc34e85064efdbb76993: {
    slug: "iso27001",
    id: "COMP-ISO27001",
    name: "ISO 27001 (Comp)",
    probo: "ISO27001-2022",
    normalize: trimId,
    title: "category-dash-description",
  },
  frk_681fdd150f59a1560a66c89a: {
    slug: "hipaa",
    id: "COMP-HIPAA",
    name: "HIPAA (Comp)",
    probo: "HIPAA",
    normalize: hipaaId,
    title: "category-dash-first-paragraph",
    minPrefix: 3,
  },
  frk_681ef1952907deb7cb85896d: {
    slug: "gdpr",
    id: "COMP-GDPR",
    name: "GDPR (Comp)",
    probo: "GDPR",
    normalize: trimId,
    title: "plain",
  },
  frk_68cc0e2a21184ed168ab2eea: {
    slug: "iso42001",
    id: "COMP-ISO42001",
    name: "ISO 42001 (Comp)",
    probo: "ISO42001-2023",
    normalize: iso42001Id,
    title: "plain",
  },
  frk_69dcfb280b550e7ee231f312: {
    slug: "pci-dss",
    id: "COMP-PCI-DSS",
    name: "PCI DSS Level 1 (Comp)",
    probo: "PCI-DSS",
    normalize: trimId,
    title: "plain",
    describe: (r) => `Evidence: ${oneLine(r.description)}`,
  },
  frk_69efd433584ba436ae00d413: {
    slug: "ccpa",
    id: "COMP-CCPA",
    name: "CCPA (Comp)",
    probo: "CCPA",
    normalize: trimId,
    title: "plain",
  },
  frk_691e28f5474c5319f1cc70d7: {
    slug: "iso9001",
    id: "ISO9001-2015",
    name: "ISO 9001 (2015)",
    probo: null,
    normalize: trimId,
    title: "plain",
  },
};

// Visible in Comp but intentionally not emitted.
const PLACEHOLDERS = {
  frk_68c1ead24950f84849db81bb:
    "visible in Comp but a placeholder: one generic requirement ('PCI Requirement'), no identifier. Superseded by 'PCI DSS Level 1' (emitted as comp-pci-dss.json).",
  frk_68e135d0212b3b6cd39ceb94:
    "visible in Comp but a placeholder: one generic requirement ('NEN 7510'), no identifier, no real clause content.",
};

// Curated semantic mappings where Comp uses its own codes instead of legal
// references (GDPR: AG-/DS-/LBT-/PR-, CCPA: CCPA-n). Comp identifier -> Probo control ids.
const CURATED = {
  // Probo uses the pre-2013 numbering for the written-contract specification.
  hipaa: {
    "164.308(b)(3)": ["164.308(b)(4)"],
  },
  gdpr: {
    "AG-1": ["Art. 24"],
    "AG-2": ["Art. 28", "Art. 28(1)", "Art. 28(3)"],
    "AG-3": ["Art. 27"],
    "AG-4": ["Art. 37", "Art. 38", "Art. 39"],
    "DS-1": ["Art. 25(1)", "Art. 25(2)"],
    "DS-2": ["Art. 5(1)(c)", "Art. 32(1)(a)"],
    "DS-3": ["Art. 32", "Art. 32(4)"],
    "DS-4": ["Art. 35(1)", "Art. 35(7)", "Art. 36"],
    "DS-5": ["Art. 33", "Art. 33(1)", "Art. 33(3)", "Art. 33(5)", "Art. 34", "Art. 34(1)"],
    "LBT-1": ["Art. 30", "Art. 30(1)"],
    "LBT-2": ["Art. 6", "Art. 7", "Art. 9(2)"],
    "LBT-3": ["Art. 12(3)", "Art. 13", "Art. 14"],
    "PR-1": ["Art. 15"],
    "PR-2": ["Art. 16"],
    "PR-3": ["Art. 17", "Art. 19"],
    "PR-4": ["Art. 18"],
    "PR-5": ["Art. 20"],
    "PR-6": ["Art. 21"],
    "PR-7": ["Art. 22"],
  },
  ccpa: {
    "CCPA-2": ["1798.130(a)(5)"],
    "CCPA-3": ["1798.105", "1798.106", "1798.110", "1798.115", "1798.120(a)", "1798.121", "1798.125(a)", "1798.130(a)(1)", "1798.130(a)(2)"],
    "CCPA-4": ["1798.130(a)(7)"],
    "CCPA-5": ["1798.130(a)(6)"],
    "CCPA-6": ["1798.100(e)"],
    "CCPA-7": ["1798.100(d)"],
    "CCPA-9": ["1798.120(a)", "1798.135(a)", "1798.135(b)"],
    "CCPA-10": ["1798.120(c)"],
    "CCPA-12": ["1798.100(a)", "1798.125(b)", "1798.135(a)"],
  },
};

// ---------------------------------------------------------------------------
// Build framework files
// ---------------------------------------------------------------------------

function controlTitle(cfg, req, normId) {
  const rawName = oneLine(req.name);
  const rawId = trimId(req.identifier);
  // Strip a leading copy of the identifier from the name ("A.5.2 Organizational Controls").
  let name = rawName;
  for (const prefix of [rawId, normId]) {
    if (prefix && name.toLowerCase().startsWith(prefix.toLowerCase())) {
      name = name.slice(prefix.length).trim();
    }
  }
  const desc = cleanText(req.description);
  if (cfg.title === "category-dash-description" && desc && desc.length <= 160 && !desc.includes("\n")) {
    return { name: `${name} - ${desc}`, description: "" };
  }
  if (cfg.title === "category-dash-first-paragraph") {
    const [first, ...rest] = desc.split(/\n\s*\n|\s{2,}/);
    const heading = (first || "").replace(/[.\s]+$/, "").trim();
    if (heading && heading.length <= 80) {
      return { name: `${name} - ${heading}`, description: rest.join("\n\n").trim() || desc };
    }
  }
  return { name: name || oneLine(req.description).slice(0, 200) || normId, description: desc };
}

const proboFiles = {};
for (const f of fs.readdirSync(proboFrameworksDir).filter((f) => f.endsWith(".json"))) {
  proboFiles[f.replace(/\.json$/, "")] = readJSON(path.join(proboFrameworksDir, f));
}

const reqsByFramework = new Map();
for (const r of compRequirements) {
  if (!reqsByFramework.has(r.frameworkId)) reqsByFramework.set(r.frameworkId, []);
  reqsByFramework.get(r.frameworkId).push(r);
}

// Comp requirement id -> { frameworkFileId, controlId }
const reqToEmitted = new Map();
const emitted = [];
const skipped = [];

fs.rmSync(outFrameworks, { recursive: true, force: true });
fs.mkdirSync(outFrameworks, { recursive: true });

const naturalCmp = (a, b) => a.localeCompare(b, "en", { numeric: true, sensitivity: "base" });

for (const fw of compFrameworks) {
  const reqs = reqsByFramework.get(fw.id) || [];
  const cfg = FRAMEWORKS[fw.id];
  if (!cfg) {
    let reason;
    if (!fw.visible) {
      reason = "hidden (visible=false)";
      if (/test|example/i.test(fw.name) || /test/i.test(fw.description)) reason += "; test/example framework";
      if (/v\.?\s*\d|v0|type 1/i.test(fw.name)) reason += "; superseded/draft version";
      if (reqs.length === 0) reason += "; no requirements";
      else if (reqs.every((r) => !trimId(r.identifier))) reason += "; requirements have no identifiers";
    } else {
      reason = PLACEHOLDERS[fw.id] || "visible but not configured in convert-comp.mjs (add it to FRAMEWORKS)";
    }
    skipped.push({ compId: fw.id, name: fw.name, version: fw.version, visible: fw.visible, requirements: reqs.length, reason });
    continue;
  }

  // Group requirements by normalized identifier; merge duplicates.
  const groups = new Map();
  for (const r of reqs) {
    const nid = cfg.normalize(r.identifier);
    if (!nid) throw new Error(`${fw.name}: requirement ${r.id} has an empty identifier`);
    if (!groups.has(nid)) groups.set(nid, []);
    groups.get(nid).push(r);
  }

  const controls = [];
  const merged = [];
  for (const nid of [...groups.keys()].sort(naturalCmp)) {
    const rs = groups.get(nid).sort((a, b) => a.id.localeCompare(b.id));
    const { name } = controlTitle(cfg, rs[0], nid);
    const descs = [];
    for (const r of rs) {
      const d = cfg.describe ? cfg.describe(r) : controlTitle(cfg, r, nid).description;
      if (d && !descs.includes(d)) descs.push(d);
    }
    if (rs.length > 1) merged.push({ id: nid, compRequirementIds: rs.map((r) => r.id) });
    controls.push({ id: nid, name, description: descs.length > 1 ? descs.map((d) => `- ${d}`).join("\n") : descs[0] || "" });
    for (const r of rs) reqToEmitted.set(r.id, { framework: cfg.id, control: nid });
  }

  const out = { id: cfg.id, name: cfg.name };
  const logoSrc = cfg.probo && proboFiles[cfg.probo]?.logo;
  if (logoSrc?.light && logoSrc?.dark) out.logo = { light: logoSrc.light, dark: logoSrc.dark };
  out.controls = controls;

  const fileName = cfg.probo ? `comp-${cfg.slug}.json` : `${cfg.slug}.json`;
  writeJSON(path.join(outFrameworks, fileName), out);
  emitted.push({
    file: `frameworks/${fileName}`,
    id: cfg.id,
    name: cfg.name,
    compId: fw.id,
    compName: fw.name,
    compVersion: fw.version,
    proboEquivalent: cfg.probo,
    requirements: reqs.length,
    controls: controls.length,
    mergedDuplicates: merged,
  });
  cfg._groups = groups;
}

// ---------------------------------------------------------------------------
// Crosswalk (Probo control -> Comp requirement)
// ---------------------------------------------------------------------------

const crosswalk = { generatedBy: "fortress/library/scripts/convert-comp.mjs", direction: "probo -> comp", frameworks: [] };

for (const cfg of Object.values(FRAMEWORKS)) {
  if (!cfg.probo || !cfg._groups) continue;
  const probo = proboFiles[cfg.probo];
  if (!probo) throw new Error(`Probo framework ${cfg.probo} not found`);
  const compIds = [...cfg._groups.keys()];
  const compByKey = new Map(compIds.map((id) => [refKey(id), id]));
  const minPrefix = cfg.minPrefix ?? 2;
  const curated = CURATED[cfg.slug] || {};
  const curatedByProbo = new Map();
  for (const [compId, proboIds] of Object.entries(curated)) {
    for (const pid of proboIds) {
      if (!probo.controls.some((c) => c.id === pid)) throw new Error(`curated map: ${cfg.slug} ${pid} not in Probo`);
      if (!curatedByProbo.has(pid)) curatedByProbo.set(pid, []);
      curatedByProbo.get(pid).push(compId);
    }
  }

  const isPrefix = (a, b) => a.length < b.length && a.every((t, i) => t === b[i]);
  const mappings = [];
  const stats = { exact: 0, ancestor: 0, descendant: 0, curated: 0, unmatched: 0 };
  const compMatched = new Set();

  for (const pc of probo.controls) {
    let matches = [];
    let type = "unmatched";
    const exact = compByKey.get(refKey(pc.id));
    if (exact) {
      matches = [exact];
      type = "exact";
    } else {
      const pt = refTokens(pc.id);
      // Closest Comp ancestor (Comp is coarser), e.g. Probo 9.2.1 -> Comp 9.2
      const anc = compIds
        .map((id) => [id, refTokens(id)])
        .filter(([, t]) => t.length >= minPrefix && isPrefix(t, pt))
        .sort((a, b) => b[1].length - a[1].length);
      if (anc.length) {
        matches = [anc[0][0]];
        type = "ancestor";
      } else if (pt.length >= minPrefix) {
        // Comp descendants (Comp is finer), e.g. Probo 9.1 -> Comp 9.1.1, 9.1.2
        const desc = compIds.filter((id) => isPrefix(pt, refTokens(id)));
        if (desc.length) {
          matches = desc.sort(naturalCmp);
          type = "descendant";
        }
      }
      if (!matches.length && curatedByProbo.has(pc.id)) {
        matches = curatedByProbo.get(pc.id).sort(naturalCmp);
        type = "curated";
      }
    }
    stats[type]++;
    matches.forEach((m) => compMatched.add(m));
    mappings.push({
      probo: pc.id,
      proboName: pc.name,
      matchType: type,
      comp: matches.map((m) => ({
        identifier: m,
        rawIdentifiers: [...new Set(cfg._groups.get(m).map((r) => r.identifier))],
        requirementIds: cfg._groups.get(m).map((r) => r.id),
      })),
    });
  }

  const matched = probo.controls.length - stats.unmatched;
  crosswalk.frameworks.push({
    proboFramework: cfg.probo,
    proboFile: `probo/apps/console/public/data/frameworks/${cfg.probo}.json`,
    compFramework: cfg.id,
    compFile: `frameworks/comp-${cfg.slug}.json`,
    method: Object.keys(curated).length
      ? "normalized reference codes, then curated map for codes that cannot match textually"
      : "normalized reference codes (exact, then hierarchical ancestor/descendant)",
    stats: {
      proboControls: probo.controls.length,
      proboMatched: matched,
      proboMatchRate: +(matched / probo.controls.length).toFixed(3),
      byType: stats,
      compRequirements: compIds.length,
      compMatched: compMatched.size,
      compMatchRate: +(compMatched.size / compIds.length).toFixed(3),
      compUnmatched: compIds.filter((id) => !compMatched.has(id)).sort(naturalCmp),
    },
    mappings,
  });
}
writeJSON(path.join(libDir, "crosswalk.json"), crosswalk);

// Comp requirement id -> Probo native refs (via the crosswalk), for process seeding.
const reqToProbo = new Map();
for (const fwx of crosswalk.frameworks) {
  for (const m of fwx.mappings) {
    for (const c of m.comp) {
      for (const rid of c.requirementIds) {
        if (!reqToProbo.has(rid)) reqToProbo.set(rid, []);
        reqToProbo.get(rid).push({ framework: fwx.proboFramework, control: m.probo, matchType: m.matchType });
      }
    }
  }
}

// ---------------------------------------------------------------------------
// Processes: control, policy and task templates
// ---------------------------------------------------------------------------

const fwById = new Map(compFrameworks.map((f) => [f.id, f]));
const reqById = new Map(compRequirements.map((r) => [r.id, r]));
const groupBy = (rows, key, val) => {
  const m = new Map();
  for (const r of rows) {
    if (!m.has(r[key])) m.set(r[key], new Set());
    m.get(r[key]).add(r[val]);
  }
  return m;
};
const ctReqs = groupBy(ctToReq, "A", "B");
const ctPols = groupBy(ctToPolicy, "A", "B");
const ctTasks = groupBy(ctToTask, "A", "B");
const polCts = groupBy(ctToPolicy, "B", "A");
const taskCts = groupBy(ctToTask, "B", "A");

const dangling = { requirementLinks: 0, policyLinks: 0, taskLinks: 0 };
const policyIds = new Set(compPolicies.map((p) => p.id));
const taskIds = new Set(compTasks.map((t) => t.id));

function requirementRef(rid) {
  const r = reqById.get(rid);
  if (!r) {
    dangling.requirementLinks++;
    return null;
  }
  const fw = fwById.get(r.frameworkId);
  const em = reqToEmitted.get(rid);
  return {
    requirementId: rid,
    compFrameworkId: r.frameworkId,
    compFramework: fw ? `${fw.name}${fw.visible ? "" : " (hidden)"}` : null,
    identifier: trimId(r.identifier) || null,
    name: oneLine(r.name),
    library: em || null, // { framework, control } in fortress/library/frameworks
    probo: reqToProbo.get(rid) || [], // native Probo framework refs via crosswalk
  };
}

const sortRefs = (refs) =>
  refs.filter(Boolean).sort(
    (a, b) =>
      (a.compFramework || "").localeCompare(b.compFramework || "") ||
      naturalCmp(a.identifier || "", b.identifier || "") ||
      a.requirementId.localeCompare(b.requirementId),
  );

const byName = (a, b) => a.name.localeCompare(b.name) || a.id.localeCompare(b.id);

const controlsOut = compControls
  .map((c) => {
    let documentTypes = c.documentTypes;
    if (typeof documentTypes === "string") documentTypes = JSON.parse(documentTypes || "[]");
    const pols = [...(ctPols.get(c.id) || [])].filter((id) => policyIds.has(id) || (dangling.policyLinks++, false));
    const tasks = [...(ctTasks.get(c.id) || [])].filter((id) => taskIds.has(id) || (dangling.taskLinks++, false));
    return {
      id: c.id,
      name: oneLine(c.name),
      description: cleanText(c.description),
      ...(c.controlFamily ? { controlFamily: c.controlFamily } : {}),
      documentTypes: documentTypes || [],
      requirements: sortRefs([...(ctReqs.get(c.id) || [])].map(requirementRef)),
      policyTemplateIds: pols.sort(),
      taskTemplateIds: tasks.sort(),
    };
  })
  .sort(byName);

const ctById = new Map(controlsOut.map((c) => [c.id, c]));
function derivedRefs(ctIds) {
  const lib = new Map();
  const probo = new Map();
  for (const id of ctIds) {
    for (const r of ctById.get(id)?.requirements || []) {
      if (r.library) lib.set(`${r.library.framework}:${r.library.control}`, r.library);
      for (const p of r.probo) probo.set(`${p.framework}:${p.control}`, { framework: p.framework, control: p.control });
    }
  }
  const s = (m) => [...m.values()].sort((a, b) => a.framework.localeCompare(b.framework) || naturalCmp(a.control, b.control));
  return { library: s(lib), probo: s(probo) };
}

function tiptapDoc(content) {
  if (Array.isArray(content)) return { type: "doc", content };
  if (content && typeof content === "object") return content;
  if (typeof content === "string") {
    try {
      return tiptapDoc(JSON.parse(content));
    } catch {
      return { type: "doc", content: [{ type: "paragraph", content: [{ type: "text", text: content }] }] };
    }
  }
  return { type: "doc", content: [] };
}

const policiesOut = compPolicies
  .map((p) => {
    const cts = [...(polCts.get(p.id) || [])].filter((id) => ctById.has(id)).sort();
    return {
      id: p.id,
      name: oneLine(p.name),
      description: cleanText(p.description),
      frequency: p.frequency,
      department: p.department,
      controlTemplateIds: cts,
      requirementRefs: derivedRefs(cts),
      contentFormat: "tiptap-json",
      content: tiptapDoc(p.content),
    };
  })
  .sort(byName);

const tasksOut = compTasks
  .map((t) => {
    const cts = [...(taskCts.get(t.id) || [])].filter((id) => ctById.has(id)).sort();
    return {
      id: t.id,
      name: oneLine(t.name),
      description: cleanText(t.description),
      frequency: t.frequency,
      department: t.department,
      automationStatus: t.automationStatus,
      controlTemplateIds: cts,
      requirementRefs: derivedRefs(cts),
    };
  })
  .sort(byName);

const header = (kind) => ({
  source: `Comp AI packages/db/prisma/seed (FrameworkEditor${kind}Template + relations)`,
  license: "AGPL-3.0 (Comp AI)",
});
writeJSON(path.join(outProcesses, "controls.json"), { ...header("Control"), count: controlsOut.length, controls: controlsOut });
writeJSON(path.join(outProcesses, "policies.json"), { ...header("Policy"), count: policiesOut.length, policies: policiesOut });
writeJSON(path.join(outProcesses, "tasks.json"), { ...header("Task"), count: tasksOut.length, tasks: tasksOut });

// ---------------------------------------------------------------------------
// Manifest
// ---------------------------------------------------------------------------

const manifest = {
  generatedBy: "fortress/library/scripts/convert-comp.mjs",
  sources: {
    comp: "packages/db/prisma/seed/{primitives,relations}",
    probo: "probo/apps/console/public/data/frameworks",
  },
  frameworks: emitted,
  skipped,
  crosswalk: crosswalk.frameworks.map((f) => ({ probo: f.proboFramework, comp: f.compFramework, ...f.stats, compUnmatched: undefined })),
  processes: {
    controls: controlsOut.length,
    controlsWithRequirements: controlsOut.filter((c) => c.requirements.length).length,
    controlsMappedToLibrary: controlsOut.filter((c) => c.requirements.some((r) => r.library)).length,
    policies: policiesOut.length,
    policiesLinked: policiesOut.filter((p) => p.controlTemplateIds.length).length,
    tasks: tasksOut.length,
    tasksLinked: tasksOut.filter((t) => t.controlTemplateIds.length).length,
    links: { controlRequirement: ctToReq.length, controlPolicy: ctToPolicy.length, controlTask: ctToTask.length },
    danglingLinks: dangling,
  },
};
writeJSON(path.join(libDir, "manifest.json"), manifest);

// Console summary
console.log("Frameworks emitted:");
for (const e of emitted) console.log(`  ${e.file.padEnd(32)} ${String(e.controls).padStart(4)} controls  (${e.compName} ${e.compVersion})`);
console.log(`Skipped: ${skipped.length}`);
for (const s of skipped) console.log(`  ${s.name} [${s.compId}]: ${s.reason}`);
console.log("Crosswalk:");
for (const f of crosswalk.frameworks) {
  const s = f.stats;
  console.log(
    `  ${f.proboFramework.padEnd(14)} <- ${f.compFramework.padEnd(14)} probo ${s.proboMatched}/${s.proboControls} (${(s.proboMatchRate * 100).toFixed(1)}%)  comp ${s.compMatched}/${s.compRequirements} (${(s.compMatchRate * 100).toFixed(1)}%)  ${JSON.stringify(s.byType)}`,
  );
}
console.log("Processes:", JSON.stringify(manifest.processes));
