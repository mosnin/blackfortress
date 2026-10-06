import { z } from 'zod';

/**
 * bf-checks reads one JSON request on stdin and writes one JSON response on
 * stdout. bfd is the only caller; it gathers credentials from the developer's
 * local CLIs (gh, aws, gcloud, az) or environment.
 */
export const runRequestSchema = z.object({
  provider: z.string().min(1),
  accessToken: z.string().optional(),
  credentials: z.record(z.string(), z.union([z.string(), z.array(z.string())])).default({}),
  variables: z.record(z.string(), z.unknown()).default({}),
  checks: z.array(z.string()).optional(),
});

export type RunRequest = z.infer<typeof runRequestSchema>;

export interface CheckOutcome {
  title: string;
  description?: string;
  resourceType: string;
  resourceId: string;
  severity?: string;
  remediation?: string;
  evidence?: unknown;
}

export interface CheckReport {
  id: string;
  name: string;
  description: string;
  taskMapping?: string;
  status: 'success' | 'failed' | 'error';
  error?: string;
  durationMs: number;
  passed: CheckOutcome[];
  findings: CheckOutcome[];
  logs: Array<{ level: string; message: string }>;
}

export interface RunResponse {
  provider: string;
  providerName: string;
  ranAt: string;
  durationMs: number;
  checks: CheckReport[];
}

export interface ProviderInfo {
  id: string;
  name: string;
  authType: string;
  checks: Array<{ id: string; name: string; description: string; taskMapping?: string }>;
}
