import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import * as acorn from 'acorn';
import { afterEach, describe, expect, test } from 'vitest';

// COMPAT-02 (issue #473 action 4 / the milestone's "unsupported
// environments fail clearly" criterion). index.html's entry point is
// `<script type="module">`, which a browser released before ES module
// support (roughly Chrome 61 / Firefox 60 / Safari 10.1 / Edge 16, all
// 2017-18 -- well under the documented Chrome/Edge/Firefox >=111,
// Safari >=16.4 floor in docs/development/supported-runtime-matrix.md)
// skips entirely and never even requests, rendering a permanently blank
// white page with zero indication why. `<script nomodule
// src="/unsupported-browser.js">` is the fix: a browser WITHOUT module
// support runs it instead, by construction (the two attributes are
// mutually exclusive per the HTML spec), so no version sniffing is needed.
//
// That module-vs-nomodule dispatch is a guaranteed browser-platform
// behavior, not something this project's code implements -- so this suite
// does not drive a real ancient browser (min-version-tests.yml's
// browser-minimum job covers the *floor* end with real pinned browsers
// instead). What this project DOES own, and what these tests cover:
//   1. index.html actually wires the nomodule fallback to the right file.
//   2. That file only uses syntax an old browser can parse in the first
//      place -- ES6+ syntax in the fallback itself would throw a
//      SyntaxError before ever reaching the DOM-manipulation logic, which
//      would silently recreate the exact blank-page failure this exists to
//      fix.
//   3. The fallback's logic, once it runs, actually replaces #root's
//      content with a comprehensible message.
const FRONTEND_ROOT = process.cwd();
const indexHtmlPath = join(FRONTEND_ROOT, 'index.html');
const fallbackScriptPath = join(FRONTEND_ROOT, 'public', 'unsupported-browser.js');

describe('COMPAT-02: pre-module-browser fallback', () => {
  afterEach(() => {
    document.body.innerHTML = '';
  });

  test('index.html wires <script nomodule> to public/unsupported-browser.js', () => {
    const html = readFileSync(indexHtmlPath, 'utf8');
    expect(html).toContain('<script type="module" src="/src/index.tsx"></script>');
    expect(html).toContain('<script nomodule src="/unsupported-browser.js"></script>');
  });

  test('public/unsupported-browser.js is plain ES5 (parseable by a pre-module browser)', () => {
    const source = readFileSync(fallbackScriptPath, 'utf8');
    // acorn defaults to reporting the earliest error a version-5 parser
    // would hit; this throws SyntaxError on arrow functions, let/const,
    // template literals, classes, etc. -- exactly the ES6+ constructs a
    // browser old enough to need this fallback cannot run either.
    expect(() => acorn.parse(source, { ecmaVersion: 5 })).not.toThrow();
  });

  test('running the fallback script replaces #root with a comprehensible message', () => {
    document.body.innerHTML = '<div id="root">should be replaced</div>';
    const source = readFileSync(fallbackScriptPath, 'utf8');
    // The "expression" here is this test's own readFileSync of a committed,
    // non-attacker-controlled file; exercising the shipped file's actual
    // logic, not a reimplementation of it, is the point.
    // biome-ignore lint/security/noGlobalEval: same rationale, see above.
    eval(source); // eslint-disable-line security/detect-eval-with-expression

    const root = document.getElementById('root');
    expect(root).not.toBeNull();
    expect(root?.innerHTML).toContain('Please update your browser');
    expect(root?.innerHTML).toContain('2023');
    expect(root?.innerHTML).not.toContain('should be replaced');
  });
});
