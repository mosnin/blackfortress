import { GetSessionTokenCommand, STSClient } from '@aws-sdk/client-sts';

type Credentials = Record<string, string | string[]>;

const LOCAL_ROLE_PLACEHOLDER = 'blackfortress-local-session';

/**
 * Comp's AWS checks normally assume a cross-account role. On a developer
 * machine there is no role to assume: bfd passes the developer's own
 * credentials (from the AWS CLI or environment). Comp's checks accept an
 * already-resolved session through underscore-prefixed fields, so this turns
 * local credentials into that shape, converting long-term keys into a
 * session with sts:GetSessionToken because the checks require a session token.
 */
export async function adaptAwsCredentials(credentials: Credentials): Promise<Credentials> {
  const accessKeyId = scalar(credentials.access_key_id);
  const secretAccessKey = scalar(credentials.secret_access_key);

  if (!accessKeyId || !secretAccessKey) return credentials;

  const regions = toRegions(credentials);
  let sessionToken = scalar(credentials.session_token);
  let resolvedKeyId = accessKeyId;
  let resolvedSecret = secretAccessKey;

  if (!sessionToken) {
    const sts = new STSClient({
      region: regions[0],
      credentials: { accessKeyId, secretAccessKey },
    });
    const response = await sts.send(new GetSessionTokenCommand({ DurationSeconds: 3600 }));
    const session = response.Credentials;

    if (!session?.AccessKeyId || !session.SecretAccessKey || !session.SessionToken) {
      throw new Error('sts:GetSessionToken returned no credentials');
    }

    resolvedKeyId = session.AccessKeyId;
    resolvedSecret = session.SecretAccessKey;
    sessionToken = session.SessionToken;
  }

  return {
    ...credentials,
    roleArn: scalar(credentials.roleArn) || LOCAL_ROLE_PLACEHOLDER,
    externalId: scalar(credentials.externalId) || LOCAL_ROLE_PLACEHOLDER,
    regions,
    __resolvedAccessKeyId: resolvedKeyId,
    __resolvedSecretAccessKey: resolvedSecret,
    __resolvedSessionToken: sessionToken,
  };
}

function scalar(value: string | string[] | undefined): string {
  if (Array.isArray(value)) return value[0] ?? '';
  return value ?? '';
}

function toRegions(credentials: Credentials): string[] {
  const raw = credentials.regions ?? credentials.region;
  const list = Array.isArray(raw) ? raw : raw ? raw.split(',') : [];
  const regions = list.map((region) => region.trim()).filter((region) => region.length > 0);

  return regions.length > 0 ? regions : ['us-east-1'];
}
