// Unit tests for the toast contrast-retry rule (nightly #2086) that decides when
// assertNoBlockingA11yViolations gives the app-wide a11y gate a second scan. The
// rule has to stay narrow -- a wrong "retry" silently weakens the gate -- so each
// disqualifying shape is pinned here. Run by vitest (e2e/*.vitest.ts).
import { afterEach, describe, expect, it } from 'vitest';
import { type AxeViolationLike, allCandidatesAreToast, toastRetryCandidates } from './a11yRetry';

const TOAST_HTML =
  '<div class="MuiPaper-root MuiAlert-root MuiAlert-filledSuccess" role="alert">Saved</div>';

function contrast(...nodes: AxeViolationLike['nodes']): AxeViolationLike {
  return { id: 'color-contrast', nodes };
}

describe('toastRetryCandidates', () => {
  it('maps a single-selector contrast node to a candidate', () => {
    expect(toastRetryCandidates([contrast({ target: ['.a'], html: '<b>' })])).toEqual([
      { selector: '.a', html: '<b>' },
    ]);
  });

  it('returns null when any violation is not color-contrast', () => {
    expect(
      toastRetryCandidates([
        contrast({ target: ['.a'], html: '<b>' }),
        { id: 'button-name', nodes: [{ target: ['.b'], html: '<button>' }] },
      ]),
    ).toBeNull();
  });

  it('returns null for a multi-part (iframe / shadow DOM) target', () => {
    expect(toastRetryCandidates([contrast({ target: ['iframe', '.a'], html: '' })])).toBeNull();
  });

  it('returns null for a non-string target part', () => {
    expect(toastRetryCandidates([contrast({ target: [['host', '.a']], html: '' })])).toBeNull();
  });

  it('returns null for an empty target', () => {
    expect(toastRetryCandidates([contrast({ target: [], html: '' })])).toBeNull();
  });
});

describe('allCandidatesAreToast', () => {
  afterEach(() => {
    document.body.innerHTML = '';
  });

  it('does not retry on an empty list', () => {
    expect(allCandidatesAreToast([])).toBe(false);
  });

  it('retries on a node still present inside a Snackbar', () => {
    document.body.innerHTML = '<div class="MuiSnackbar-root"><p id="msg">Saved</p></div>';
    expect(allCandidatesAreToast([{ selector: '#msg', html: '<p id="msg">' }])).toBe(true);
  });

  it('does not retry on a node still present outside a Snackbar', () => {
    document.body.innerHTML = '<main><p id="msg">Saved</p></main>';
    expect(allCandidatesAreToast([{ selector: '#msg', html: TOAST_HTML }])).toBe(false);
  });

  it('retries on a gone node whose recorded html was a toast', () => {
    expect(allCandidatesAreToast([{ selector: '#gone', html: TOAST_HTML }])).toBe(true);
    expect(
      allCandidatesAreToast([
        {
          selector: '#gone',
          html: '<div class="MuiSnackbar-root MuiSnackbar-anchorOriginBottomLeft">',
        },
      ]),
    ).toBe(true);
  });

  it('does not retry on a gone node whose recorded html was not a toast', () => {
    expect(
      allCandidatesAreToast([{ selector: '#gone', html: '<p class="MuiTypography-root">x</p>' }]),
    ).toBe(false);
  });

  it('does not retry when only one of several nodes qualifies', () => {
    document.body.innerHTML = '<div class="MuiSnackbar-root"><p id="msg">Saved</p></div>';
    expect(
      allCandidatesAreToast([
        { selector: '#msg', html: TOAST_HTML },
        { selector: '#gone', html: '<p class="MuiTypography-root">x</p>' },
      ]),
    ).toBe(false);
  });

  it('retries when every node qualifies', () => {
    document.body.innerHTML = '<div class="MuiSnackbar-root"><p id="msg">Saved</p></div>';
    expect(
      allCandidatesAreToast([
        { selector: '#msg', html: TOAST_HTML },
        { selector: '#gone', html: TOAST_HTML },
      ]),
    ).toBe(true);
  });

  it('does not retry on an unusable selector', () => {
    expect(allCandidatesAreToast([{ selector: '###', html: TOAST_HTML }])).toBe(false);
    expect(allCandidatesAreToast([{ selector: null, html: TOAST_HTML }])).toBe(false);
  });
});
