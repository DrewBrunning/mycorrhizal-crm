// Skip-as-pass guard for e2e specs that need a tool the CI job provisions
// (issue #1315) -- the Playwright counterpart of the Go
// internal/citest.SkipOrRequire helper. Same switch: MYCORRHIZAL_REQUIRE_REFERENCES=1
// (set in e2e-tests.yml), with a generic CI=<truthy> as a backstop so any CI
// runner fails loudly too. Locally neither is set, so a missing tool skips.
//
// Not named *.spec.ts / *.test.ts on purpose: Playwright would collect those.
// Its unit test is requireCiTool.vitest.ts (run by vitest, see vitest.config.ts).

type Env = Record<string, string | undefined>;

export function ciRequiresTools(env: Env = process.env): boolean {
  if (env.MYCORRHIZAL_REQUIRE_REFERENCES === '1') return true;
  const ci = env.CI;
  return ci !== undefined && ci !== '' && ci !== '0' && ci.toLowerCase() !== 'false';
}

/**
 * Returns true when the tool is available (run the test). When it is missing:
 * throws in CI (so a broken provisioning step turns the job red) and returns
 * false locally (the caller then calls test.skip with `message`).
 */
export function requireToolOrSkip(
  available: boolean,
  message: string,
  env: Env = process.env,
): boolean {
  if (available) return true;
  if (ciRequiresTools(env)) {
    throw new Error(`required CI tool missing (skip-as-pass is disabled in CI): ${message}`);
  }
  return false;
}
