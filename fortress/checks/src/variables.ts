import type { CheckVariableValues, IntegrationManifest } from '../../../packages/integration-platform/src/types';

/**
 * Builds the variable values for a run: each declared variable's default
 * (integration-level, then check-level), overlaid with the caller's values.
 */
export function resolveVariables({
  manifest,
  checkIds,
  overrides,
}: {
  manifest: IntegrationManifest;
  checkIds: string[];
  overrides: Record<string, unknown>;
}): CheckVariableValues {
  const values: CheckVariableValues = {};

  const declared = [
    ...(manifest.variables ?? []),
    ...(manifest.checks ?? [])
      .filter((check) => checkIds.includes(check.id))
      .flatMap((check) => check.variables ?? []),
  ];

  for (const variable of declared) {
    if (variable.default !== undefined && values[variable.id] === undefined) {
      values[variable.id] = variable.default;
    }
  }

  for (const [key, value] of Object.entries(overrides)) {
    if (isVariableValue(value)) values[key] = value;
  }

  return values;
}

function isVariableValue(value: unknown): value is CheckVariableValues[string] {
  if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') {
    return true;
  }

  return Array.isArray(value) && value.every((item) => typeof item === 'string');
}
