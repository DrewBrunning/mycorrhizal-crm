import { readFileSync } from 'node:fs';
import jsxA11y from 'eslint-plugin-jsx-a11y';
import react from 'eslint-plugin-react';
import reactHooks from 'eslint-plugin-react-hooks';
import security from 'eslint-plugin-security';
import tseslint from 'typescript-eslint';

// Issue #2920 item 3: existing hand-assembled month/day sites, counted and
// reasoned. The list is the same committed file `scripts/check-month-day-format.mjs`
// ratchets down (a new site in an allowlisted file still fails that script).
const monthDayAllowlist = JSON.parse(
  readFileSync(new URL('./month-day-allowlist.json', import.meta.url), 'utf8'),
);
const monthDayAllowlistedFiles = Object.keys(monthDayAllowlist.files);
const monthDayExemptFiles = ['src/DateFormatProvider.tsx', ...monthDayAllowlistedFiles];

// Issue #2920 item 1: files that already contain raw `err.message` sites are
// covered by the counted allowlist (one entry per file, checked by
// scripts/check-raw-error-message.mjs). The rule below applies to everything
// else in src/, so a new offender cannot land.
const rawErrorAllowlist = JSON.parse(
  readFileSync(new URL('./raw-error-message-allowlist.json', import.meta.url), 'utf8'),
);
const rawErrorExemptFiles = [
  'src/**/*.test.*',
  'src/**/*.spec.*',
  'src/setupTests.ts',
  ...Object.keys(rawErrorAllowlist),
];

// The two #2920 guards both use `no-restricted-syntax`, but ESLint replaces a
// rule's whole option list when a later config object matches the same file
// (it does not append). The config objects below therefore run in three
// passes: each guard alone for the files exempt from the other guard, then
// both together for everything else -- otherwise the second object would
// silently drop the first selector.
const monthDaySelector = {
  selector: 'TemplateLiteral:has(Identifier[name=/month/i]):has(Identifier[name=/day/i])',
  message:
    'Do not assemble a month and day by hand. Use formatMonthDay(month, day, locale) from DateFormatProvider (#2920).',
};
const rawErrorMessageSelector = {
  selector:
    "ConditionalExpression[test.type='BinaryExpression'][test.operator='instanceof'][test.right.type='Identifier'][test.right.name='Error'][consequent.type='MemberExpression'][consequent.property.type='Identifier'][consequent.property.name='message']",
  message: 'Use getErrorMessage(err) from utils/errorHandler (#1915)',
};

export default tseslint.config(
  {
    ignores: [
      'build/**',
      'dist/**',
      'node_modules/**',
      'playwright-report/**',
      'test-results/**',
      'coverage/**',
      // Issue #476 (WEB-02): the sw-upgrade suite generates real production
      // builds under e2e/sw-upgrade/fixtures and its own Playwright artifacts.
      'e2e/sw-upgrade/fixtures/**',
      'playwright-report-sw/**',
      'test-results-sw/**',
      // Generated from backend/openapi.yaml by `go run ./cmd/gentsapi`; never hand-edited.
      'src/generated/**',
    ],
  },
  tseslint.configs.recommended,
  {
    // Type-aware linting (typescript-eslint's projectService) needs a real
    // tsconfig program per file. `tsconfig.json` covers `src/**`;
    // `tsconfig.node.json` covers `e2e/**` plus the root-level config files
    // (vite/vitest/playwright configs). projectService discovers both
    // automatically via directory walk, but `eslint.config.js` itself and
    // other files outside both tsconfigs' `include` need `allowDefaultProject`
    // or they'd hard-error — listed explicitly below.
    //
    // Deliberately NOT `tseslint.configs.recommendedTypeChecked`: that pulls
    // in the whole `no-unsafe-*`/`require-await`/`no-unnecessary-type-assertion`
    // family, which surfaced ~2200 more findings across this codebase (mostly
    // `no-unsafe-member-access`/`no-unsafe-assignment` from the api/ layer's
    // deliberate `Record<string, any>` parsing, already covered by the
    // existing `no-explicit-any: warn` policy above) — a separate, much
    // larger cleanup than the promise-safety gap this change targets. Only
    // the specific type-aware rules below are turned on.
    files: ['**/*.{ts,tsx}'],
    languageOptions: {
      parserOptions: {
        projectService: {
          allowDefaultProject: ['eslint.config.js', 'playwright.sw.config.ts'],
        },
        tsconfigRootDir: import.meta.dirname,
      },
    },
  },
  {
    files: ['**/*.{ts,tsx}'],
    plugins: {
      'react-hooks': reactHooks,
      react,
      security,
      'jsx-a11y': jsxA11y,
    },
    rules: {
      // WCAG audit (#148) gate: jsx-a11y catches the authoring-time class of
      // findings (unnamed icon buttons, unlabeled controls) the same way
      // TypeScript catches types. Recommended config, minus the rules
      // downgraded below — never disable the plugin outright.
      ...jsxA11y.configs.recommended.rules,
      // The recommended config only checks controls whose role is alert or
      // dialog, and only raw DOM elements — neither of which covers MUI's
      // <IconButton>/<Button> components, so it would NOT have caught the
      // audit's unnamed AppBar buttons / dialog close / info-popover triggers.
      // Extend includeRoles to button/link (their implicit roles) and list the
      // MUI control components as controlComponents. `*Icon` must be listed
      // too: mayHaveAccessibleLabel assumes any React-component child could be
      // a label, so without it a bare <EditIcon/> child makes the rule treat
      // the button as labeled. The 36 findings this surfaced were all genuine.
      'jsx-a11y/control-has-associated-label': [
        'error',
        {
          includeRoles: ['alert', 'dialog', 'button', 'link'],
          controlComponents: ['IconButton', 'Button', '*Icon', '*IconButton'],
        },
      ],
      // Deliberately `warn`, not `error`: all 20 existing hits are
      // interaction-triggered focus (the first field of a just-opened dialog,
      // or a just-activated inline edit), which is correct keyboard a11y —
      // the WCAG concern is *page-load* autofocus, and none of these are
      // that. Kept visible so new page-load autofocus is reviewed, matching
      // the repo's pattern for exhaustive-deps / no-explicit-any.
      //
      // "Visible" is enforced, not hoped for: `yarn lint` runs with
      // --max-warnings at the current warn-level count (package.json), so a
      // NEW warning fails CI and the pre-commit hook. A PR that removes
      // warnings lowers that number in the same change; raising it is a
      // reviewed diff with a reason (v1.5.0 review: these used to pass
      // silently).
      'jsx-a11y/no-autofocus': 'warn',
      'react-hooks/rules-of-hooks': 'error',
      'react-hooks/exhaustive-deps': 'warn',
      // Browser XSS / tabnabbing guards.
      'react/no-danger': 'error',
      'react/jsx-no-target-blank': 'error',
      'security/detect-eval-with-expression': 'error',
      'security/detect-non-literal-regexp': 'error',
      'security/detect-unsafe-regex': 'error',
      // The api/ layer parses responses as Record<string, any>; tightening it
      // is a separate refactor. Keep `any` visible (warning) but non-blocking.
      '@typescript-eslint/no-explicit-any': 'warn',
      // `_`-prefixed bindings are deliberately ignored (destructure-and-ignore).
      '@typescript-eslint/no-unused-vars': [
        'error',
        { argsIgnorePattern: '^_', varsIgnorePattern: '^_' },
      ],
      // Type-aware promise-safety rules (already 'error' in
      // recommendedTypeChecked; restated explicitly since they're the point
      // of enabling projectService). checksVoidReturn stays fully on
      // (default): MUI/DOM event handlers (onClick etc.) are typed to expect
      // a void return, and an async handler that silently swallows its own
      // rejection is exactly the flake/lost-error class this rule exists to
      // catch — `attributes: false` was considered and rejected here because
      // this repo's handlers are the common offender, not an edge case.
      '@typescript-eslint/no-floating-promises': 'error',
      '@typescript-eslint/no-misused-promises': 'error',
      '@typescript-eslint/await-thenable': 'error',
    },
  },
  {
    // Issue #2920 item 3: a year-less month/day is display text, so Intl must
    // own its order and punctuation ("March 3" vs "3. März"). Ban the
    // hand-assembled template literal; DateFormatProvider.tsx owns the
    // formatters (and its existing numeric/MONTHS_* builders are exempt).
    // Existing sites live in month-day-allowlist.json, whose exact counts
    // scripts/check-month-day-format.mjs enforces.
    //
    // This pass covers the files the raw-error guard exempts (tests and
    // allowlisted files) so the month-day rule still applies there.
    files: ['src/**/*.tsx'],
    ignores: [...monthDayExemptFiles],
    rules: {
      'no-restricted-syntax': ['error', monthDaySelector],
    },
  },
  {
    // Issue #2920 item 1 / #1915: a raw `err.message` in UI text ("Failed to
    // fetch", a server-internal string) tells the user nothing.
    // getErrorMessage() (src/utils/errorHandler.ts) maps network failures and
    // ApiError responses to a translated, user-safe message. Files with
    // existing sites are exempt here and ratcheted by count in
    // raw-error-message-allowlist.json; test/spec/setup files render no UI
    // text. A selector cannot compare `X` across nodes, so this matches any
    // `… instanceof Error ? … .message` ternary (all current sites share X).
    files: ['src/**/*.{ts,tsx}'],
    ignores: [...rawErrorExemptFiles],
    rules: {
      'no-restricted-syntax': ['error', rawErrorMessageSelector],
    },
  },
  {
    // Files subject to both #2920 guards: carry both selectors, since a later
    // matching object replaces the whole `no-restricted-syntax` option list.
    files: ['src/**/*.tsx'],
    ignores: [...monthDayExemptFiles, ...rawErrorExemptFiles],
    rules: {
      'no-restricted-syntax': ['error', monthDaySelector, rawErrorMessageSelector],
    },
  },
  {
    // Playwright specs legitimately build RegExps from variables in assertions.
    files: ['e2e/**'],
    rules: {
      'security/detect-non-literal-regexp': 'off',
    },
  },
  {
    // fixtures.ts's `page` fixture override (issue #259's automatic a11y
    // scan) has a `(fixtures, use, testInfo) => {...}` signature per
    // Playwright's own fixture convention. eslint-plugin-react-hooks
    // pattern-matches any function param literally named `use` as a React
    // Hook call and misfires rules-of-hooks on it — this file has no JSX and
    // no actual React hooks, so the rule has nothing real to protect here.
    files: ['e2e/fixtures.ts'],
    rules: {
      'react-hooks/rules-of-hooks': 'off',
    },
  },
);
