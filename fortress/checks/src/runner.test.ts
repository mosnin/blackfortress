import { describe, expect, test } from 'bun:test';
import { adaptAwsCredentials } from './aws';
import { listProviders, runRequest } from './main';
import { runRequestSchema } from './protocol';
import { resolveVariables } from './variables';
import { getManifest } from '../../../packages/integration-platform/src/registry';

describe('listProviders', () => {
  test('exposes every provider that has checks, with task mappings', () => {
    const providers = listProviders();
    const ids = providers.map((p) => p.id);

    expect(ids).toContain('github');
    expect(ids).toContain('aws');
    expect(ids).not.toContain('rippling');

    const github = providers.find((p) => p.id === 'github');
    expect(github?.checks.some((c) => c.taskMapping)).toBe(true);
  });
});

describe('resolveVariables', () => {
  test('applies declared defaults, then caller overrides', () => {
    const manifest = getManifest('github');
    if (!manifest) throw new Error('github manifest missing');

    const values = resolveVariables({
      manifest,
      checkIds: ['dependabot_enabled', 'branch_protection'],
      overrides: { target_repos: ['acme/api:main'], ignored: { nested: true } },
    });

    expect(values.target_repos).toEqual(['acme/api:main']);
    expect(values.ignored).toBeUndefined();
  });
});

describe('adaptAwsCredentials', () => {
  test('passes session credentials through without calling STS', async () => {
    const adapted = await adaptAwsCredentials({
      access_key_id: 'AKIAEXAMPLE',
      secret_access_key: 'secret',
      session_token: 'token',
      regions: ['eu-west-1', 'us-east-1'],
    });

    expect(adapted.__resolvedAccessKeyId).toBe('AKIAEXAMPLE');
    expect(adapted.__resolvedSessionToken).toBe('token');
    expect(adapted.regions).toEqual(['eu-west-1', 'us-east-1']);
    expect(adapted.roleArn).toBeTruthy();
    expect(adapted.externalId).toBeTruthy();
  });

  test('leaves non-key credentials untouched', async () => {
    const input = { roleArn: 'arn:aws:iam::1:role/x', externalId: 'e', regions: ['us-east-1'] };
    expect(await adaptAwsCredentials(input)).toEqual(input);
  });
});

describe('runRequest', () => {
  test('rejects unknown providers', async () => {
    const req = runRequestSchema.parse({ provider: 'nope' });
    await expect(runRequest(req)).rejects.toThrow('unknown provider');
  });

  test('turns a failed API call into findings, not a crash', async () => {
    const original = globalThis.fetch;
    globalThis.fetch = (async () =>
      new Response(JSON.stringify({ message: 'Bad credentials' }), {
        status: 401,
        headers: { 'content-type': 'application/json' },
      })) as unknown as typeof fetch;

    try {
      const res = await runRequest(
        runRequestSchema.parse({
          provider: 'github',
          accessToken: 'invalid',
          variables: { target_repos: ['acme/api:main'] },
          checks: ['branch_protection'],
        }),
      );

      expect(res.checks).toHaveLength(1);
      expect(['failed', 'error']).toContain(res.checks[0].status);
    } finally {
      globalThis.fetch = original;
    }
  });
});
