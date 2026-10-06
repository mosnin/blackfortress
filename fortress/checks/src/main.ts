import { getAllManifests, getManifest } from '../../../packages/integration-platform/src/registry';
import { runCheck } from '../../../packages/integration-platform/src/runtime';
import type { IntegrationManifest } from '../../../packages/integration-platform/src/types';
import { adaptAwsCredentials } from './aws';
import {
  runRequestSchema,
  type CheckOutcome,
  type CheckReport,
  type ProviderInfo,
  type RunRequest,
  type RunResponse,
} from './protocol';
import { resolveVariables } from './variables';

const MAX_EVIDENCE_BYTES = 32 * 1024;

const usage = `bf-checks — run Comp integration checks locally

Usage:
  bf-checks list           Print providers and their checks as JSON
  bf-checks run < req.json Run checks; request and response are JSON
`;

export function listProviders(): ProviderInfo[] {
  return getAllManifests()
    .filter((manifest) => (manifest.checks ?? []).length > 0)
    .map((manifest) => ({
      id: manifest.id,
      name: manifest.name,
      authType: manifest.auth.type,
      checks: (manifest.checks ?? []).map((check) => ({
        id: check.id,
        name: check.name,
        description: check.description,
        taskMapping: check.taskMapping,
      })),
    }));
}

export async function runRequest(request: RunRequest): Promise<RunResponse> {
  const manifest = getManifest(request.provider);
  if (!manifest) throw new Error(`unknown provider "${request.provider}"`);

  const checks = (manifest.checks ?? []).filter(
    (check) => !request.checks || request.checks.includes(check.id),
  );

  const credentials =
    manifest.id === 'aws' ? await adaptAwsCredentials(request.credentials) : request.credentials;

  const variables = resolveVariables({
    manifest,
    checkIds: checks.map((check) => check.id),
    overrides: request.variables,
  });

  const started = Date.now();
  const reports: CheckReport[] = [];

  for (const check of checks) {
    const result = await runCheck(check, {
      manifest,
      accessToken: request.accessToken,
      credentials,
      variables,
      connectionId: `local-${manifest.id}`,
      organizationId: 'local',
      logger: stderrLogger(manifest),
    });

    reports.push({
      id: check.id,
      name: check.name,
      description: check.description,
      taskMapping: check.taskMapping,
      status: result.status,
      error: result.error,
      durationMs: result.durationMs,
      passed: result.result.passingResults.map(toOutcome),
      findings: result.result.findings.map(toOutcome),
      logs: result.result.logs.map((log) => ({ level: log.level, message: log.message })),
    });
  }

  return {
    provider: manifest.id,
    providerName: manifest.name,
    ranAt: new Date(started).toISOString(),
    durationMs: Date.now() - started,
    checks: reports,
  };
}

function toOutcome(result: {
  title: string;
  description?: string;
  resourceType: string;
  resourceId: string;
  severity?: string;
  remediation?: string;
  evidence?: unknown;
}): CheckOutcome {
  return {
    title: result.title,
    description: result.description,
    resourceType: result.resourceType,
    resourceId: result.resourceId,
    severity: result.severity,
    remediation: result.remediation,
    evidence: capEvidence(result.evidence),
  };
}

function capEvidence(evidence: unknown): unknown {
  if (evidence === undefined) return undefined;

  const json = JSON.stringify(evidence);
  if (json === undefined || json.length <= MAX_EVIDENCE_BYTES) return evidence;

  return { truncated: true, bytes: json.length, preview: json.slice(0, MAX_EVIDENCE_BYTES) };
}

function stderrLogger(manifest: IntegrationManifest) {
  const write = (level: string) => (message: string) => {
    process.stderr.write(`[${manifest.id}] ${level} ${message}\n`);
  };

  return { info: write('info'), warn: write('warn'), error: write('error') };
}

async function main(): Promise<number> {
  const command = process.argv[2];

  if (command === 'list') {
    process.stdout.write(JSON.stringify(listProviders()) + '\n');
    return 0;
  }

  if (command !== 'run') {
    process.stderr.write(usage);
    return 2;
  }

  const raw = await Bun.stdin.text();
  const parsed = runRequestSchema.safeParse(JSON.parse(raw));
  if (!parsed.success) {
    process.stderr.write(`invalid request: ${parsed.error.message}\n`);
    return 2;
  }

  const response = await runRequest(parsed.data);
  process.stdout.write(JSON.stringify(response) + '\n');

  return 0;
}

if (import.meta.main) {
  main()
    .then((code) => process.exit(code))
    .catch((error: unknown) => {
      process.stderr.write(`bf-checks: ${error instanceof Error ? error.message : String(error)}\n`);
      process.exit(1);
    });
}
