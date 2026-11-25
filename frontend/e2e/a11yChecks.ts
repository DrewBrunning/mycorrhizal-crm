// Pure rules behind the shared accessibility gate in fixtures.ts.
//
// Kept free of Playwright runtime imports so vitest can unit-test them
// (a11yChecks.vitest.ts): a wrong filter silently weakens the app-wide gate,
// and a wrong repeated-name rule either misses #2911's class or floods CI with
// false positives.

/** An axe violation as much as the filter needs. Structurally compatible. */
export interface A11yViolationLike {
  id: string;
  impact?: string | null;
}

/** Blocking impact levels -- shared with accessibility.spec.ts so the two never drift. */
export const BLOCKING_A11Y_IMPACTS = ['critical', 'serious'];

/**
 * axe's WCAG 2.0/2.1/2.2 A+AA tags the #148 audit ran.
 *
 * `label-content-name-mismatch` is tagged wcag21a in axe-core 4.14 and so is
 * already covered here; it is still named in EXTRA_A11Y_RULES so an axe upgrade
 * that moves it back under `experimental` (its older tag) cannot silently drop
 * #2906 again.
 */
export const WCAG_A11Y_TAGS = ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa'];

/**
 * Rules outside WCAG_A11Y_TAGS that this gate nevertheless blocks on, at any
 * impact:
 * - `heading-order` is best-practice, not WCAG-tagged. #2910's import dialog
 *   jumps h2 -> h6 and axe reports it as `moderate`; the old gate only failed
 *   `critical`/`serious`, so it never saw the skip.
 * - `label-content-name-mismatch` reports #2906's hamburger as `serious`, but
 *   naming it here is the upgrade-stable half of the reason above.
 */
export const EXTRA_A11Y_RULES = ['heading-order', 'label-content-name-mismatch'];

/**
 * True when an axe violation must fail the gate: the usual critical/serious
 * impacts, or any impact at all for EXTRA_A11Y_RULES.
 */
export function isBlockingA11yViolation(violation: A11yViolationLike): boolean {
  return (
    EXTRA_A11Y_RULES.includes(violation.id) ||
    BLOCKING_A11Y_IMPACTS.includes(violation.impact ?? '')
  );
}

/**
 * One pre-existing axe node this gate would otherwise fail on. `htmlIncludes`
 * matches a stable fragment of axe's `node.html` (never an emotion class name),
 * so a genuinely new offender still fails. Rendered counts are data-dependent
 * and axe reports the first skip per subtree, so these are fragment matches,
 * not counted entries.
 */
export interface A11yViolationNodeExemption {
  rule: string;
  htmlIncludes: string;
  reason: string;
}

export const A11Y_VIOLATION_NODE_EXEMPTIONS: A11yViolationNodeExemption[] = [
  {
    rule: 'heading-order',
    htmlIncludes: '>Circles</h6>',
    reason:
      'AddContactDialog renders its Circles section label as h6 after the dialog h2; found by this gate, not one of the #2906-#2919 fixes',
  },
];

/** True when an axe node is a committed existing offender for `rule`. */
export function isExemptA11yViolationNode(rule: string, html: string): boolean {
  return A11Y_VIOLATION_NODE_EXEMPTIONS.some(
    (e) => e.rule === rule && html.includes(e.htmlIncludes),
  );
}

// ---------------------------------------------------------------------------
// Repeated accessible names (#2911)
// ---------------------------------------------------------------------------
// axe has no "two controls in one region share a name" rule. This is the
// complement of `label-content-name-mismatch`: that one catches a name that
// omits visible content, this one catches distinct actions that collapse to the
// same name ("Merge", "Not a duplicate", four "Add" buttons).

/** The accessibility nodes the walker reads (a subset of ariaSnapshotJSON). */
export interface A11yNode {
  role?: string;
  name?: string;
  children?: Array<A11yNode | string>;
}

/** Containers whose controls must be individually nameable. */
export const REPEATED_NAME_SCOPES = ['dialog', 'main', 'list', 'grid'] as const;
export type RepeatedNameScope = (typeof REPEATED_NAME_SCOPES)[number];

/**
 * One committed exemption. `scopes` is deliberately narrow: a row-scoped
 * "Remove" is allowed in a list/grid, but the same repeated name in a dialog is
 * still a finding. The occurrence count is data-dependent (one per row), so
 * this is not a counted allowlist; everything not named here still fails.
 */
export interface RepeatedNameExemption {
  name: string;
  scopes: RepeatedNameScope[];
  reason: string;
}

export const REPEATED_NAME_EXEMPTIONS: RepeatedNameExemption[] = [
  {
    name: 'Remove',
    scopes: ['list', 'grid'],
    reason: 'row-scoped destructive action; the row around it supplies the item context',
  },
];

export interface RepeatedNameFinding {
  scope: RepeatedNameScope;
  name: string;
  count: number;
}

function isScope(role: string | undefined): role is RepeatedNameScope {
  return role !== undefined && (REPEATED_NAME_SCOPES as readonly string[]).includes(role);
}

function isAction(role: string | undefined): boolean {
  return role === 'button' || role === 'link';
}

function nameKey(name: string): string {
  return name.trim();
}

function isExempt(name: string, scope: RepeatedNameScope): boolean {
  return REPEATED_NAME_EXEMPTIONS.some((e) => e.name === name && e.scopes.includes(scope));
}

/**
 * Collects button/link names in one scope. Controls inside a nested scope
 * belong to that nested scope, not this one -- otherwise a repeated name in a
 * dialog's list would be reported twice (once for the list, once for the
 * dialog) and a fix to either would look like a regression in the other.
 */
function collectScopeNames(scope: A11yNode): Map<string, number> {
  const counts = new Map<string, number>();
  const walk = (nodes: Array<A11yNode | string> | undefined): void => {
    for (const node of nodes ?? []) {
      if (typeof node === 'string') continue;
      if (isScope(node.role)) continue;
      if (isAction(node.role) && node.name && nameKey(node.name).length > 0) {
        const name = nameKey(node.name);
        counts.set(name, (counts.get(name) ?? 0) + 1);
      }
      walk(node.children);
    }
  };
  walk(scope.children);
  return counts;
}

/**
 * Walks an accessibility snapshot and returns every repeated action name that
 * is not exempt in its scope.
 */
export function findRepeatedAccessibleNames(
  nodes: Array<A11yNode | string>,
): RepeatedNameFinding[] {
  const findings: RepeatedNameFinding[] = [];

  const visit = (list: Array<A11yNode | string> | undefined): void => {
    for (const node of list ?? []) {
      if (typeof node === 'string') continue;
      if (isScope(node.role)) {
        for (const [name, count] of collectScopeNames(node)) {
          if (count > 1 && !isExempt(name, node.role)) {
            findings.push({ scope: node.role, name, count });
          }
        }
      }
      visit(node.children);
    }
  };

  visit(nodes);
  return findings;
}

/**
 * True when every same-named control in `descriptions` carries its own,
 * non-empty description: `aria-describedby` has made the names distinct, the
 * documented escape hatch. A single missing or shared description is not
 * enough -- that is the pre-fix shape the check exists to catch.
 */
export function describedByDistinguishes(descriptions: Array<string | null>): boolean {
  if (descriptions.length < 2) return false;
  const normalized = descriptions.map((d) => (d ?? '').trim());
  if (normalized.some((d) => d.length === 0)) return false;
  return new Set(normalized).size === normalized.length;
}
