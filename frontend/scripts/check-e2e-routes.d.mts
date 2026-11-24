// Types for check-e2e-routes.mjs. The script is plain ESM so the route-coverage
// guard runs without a build step; the pseudo-locale pass imports loadRoutes so
// both checks read the same App.tsx table (issue #2920).
export const APP_FILE: string;
export const E2E_DIR: string;
export const EXEMPTIONS_FILE: string;

export function parseRoutes(appSource: string): string[];
export function loadRoutes(file?: string): string[];
export function routeRegex(routePath: string): RegExp;
export function isCovered(routePath: string, specSources: string[]): boolean;
export function checkRoutes(
  routes: string[],
  specSources: string[],
  exemptions: Record<string, string>,
): { ok: boolean; uncovered: string[]; stale: string[]; badReasons: string[] };
export function loadSpecSources(dir?: string): string[];
export function loadExemptions(file?: string): Record<string, string>;
export function run(): number;
