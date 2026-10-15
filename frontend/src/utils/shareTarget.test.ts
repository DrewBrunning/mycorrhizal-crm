import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { makeFakeCaches } from '../test/fakeCaches';
import {
  composeSharedDraft,
  SHARE_TEXT_MAX_CHARS,
  sanitizeSharedText,
  storeSharedDraft,
  takeSharedDraft,
} from './shareTarget';

describe('sanitizeSharedText', () => {
  it('strips control and format characters but keeps newline and tab', () => {
    expect(sanitizeSharedText('a\u0000b​c‮d\ne\tf\r')).toBe('abcd\ne\tf');
  });

  it('returns null for non-strings and for text with nothing left', () => {
    expect(sanitizeSharedText(null)).toBeNull();
    expect(sanitizeSharedText(new File([], 'x'))).toBeNull();
    expect(sanitizeSharedText(' ​\n ')).toBeNull();
  });

  it('truncates past the limit and marks the cut', () => {
    const exact = 'x'.repeat(SHARE_TEXT_MAX_CHARS);
    expect(sanitizeSharedText(exact)).toBe(exact);
    expect(sanitizeSharedText(`${exact}y`)).toBe(`${exact}…`);
  });
});

describe('composeSharedDraft', () => {
  it('joins title, text and url with blank lines', () => {
    expect(composeSharedDraft('T', 'body', 'https://x.test')).toBe('T\n\nbody\n\nhttps://x.test');
  });

  it('skips empty parts and a url already inside the text', () => {
    expect(composeSharedDraft('', 'see https://x.test', 'https://x.test')).toBe(
      'see https://x.test',
    );
    expect(composeSharedDraft(null, '\u0000', ' ')).toBeNull();
  });
});

describe('storeSharedDraft / takeSharedDraft', () => {
  beforeEach(() => {
    vi.stubGlobal('caches', makeFakeCaches());
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('hands the draft over exactly once', async () => {
    const key = await storeSharedDraft(caches, window.location.origin, 'hello');
    expect(await takeSharedDraft(key)).toBe('hello');
    expect(await takeSharedDraft(key)).toBeNull();
  });

  it('keeps only the newest pending draft', async () => {
    const first = await storeSharedDraft(caches, window.location.origin, 'one');
    const second = await storeSharedDraft(caches, window.location.origin, 'two');
    expect(await takeSharedDraft(first)).toBeNull();
    expect(await takeSharedDraft(second)).toBe('two');
  });

  it('rejects missing, malformed and unknown keys', async () => {
    expect(await takeSharedDraft(null)).toBeNull();
    expect(await takeSharedDraft('../../etc')).toBeNull();
    expect(await takeSharedDraft('00000000-0000-4000-8000-000000000000')).toBeNull();
  });

  it('is null when the Cache API is unavailable', async () => {
    vi.stubGlobal('caches', undefined);
    expect(await takeSharedDraft('00000000-0000-4000-8000-000000000000')).toBeNull();
  });
});
