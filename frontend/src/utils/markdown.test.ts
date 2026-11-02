import { describe, expect, it } from 'vitest';
import { MARKDOWN_MAX_LENGTH, parseInlineMarkdown, parseMarkdown, sanitizeUrl } from './markdown';

describe('sanitizeUrl', () => {
  it.each([
    'javascript:alert(1)',
    'JaVaScRiPt:alert(1)',
    'java\tscript:alert(1)',
    ' \u0001javascript:alert(1)',
    'data:text/html,<script>alert(1)</script>',
    'vbscript:msgbox(1)',
    '//evil.example/x',
    '/relative',
    'file:///etc/passwd',
    '',
  ])('rejects %j', (u) => {
    expect(sanitizeUrl(u)).toBeNull();
  });

  it.each(['https://example.com/a?b=1', 'http://example.com', 'mailto:a@b.co', 'tel:+15551234'])(
    'allows %s',
    (u) => {
      expect(sanitizeUrl(u)).toBe(u);
    },
  );
});

describe('parseInlineMarkdown', () => {
  it('leaves plain text and raw HTML as literal text', () => {
    const src = '<img src=x onerror=alert(1)> <script>alert(1)</script> a < b';
    expect(parseInlineMarkdown(src)).toEqual([{ kind: 'text', text: src }]);
  });

  it('does not link a javascript: URL; the source stays visible', () => {
    const out = parseInlineMarkdown('[x](javascript:alert(1))');
    expect(out.some((n) => n.kind === 'link')).toBe(false);
  });

  it('keeps image syntax as literal text', () => {
    expect(parseInlineMarkdown('![a](https://e.com/x.png)')).toEqual([
      { kind: 'text', text: '![a](https://e.com/x.png)' },
    ]);
  });

  it('does not italicise snake_case or math-like asterisks', () => {
    expect(parseInlineMarkdown('snake_case_word 2 * 3 * 4')).toEqual([
      { kind: 'text', text: 'snake_case_word 2 * 3 * 4' },
    ]);
  });

  it('handles unterminated and nested markers without throwing', () => {
    for (const s of [
      '**bold',
      '[a](',
      '[[[[',
      '`x',
      '*a **b* c**',
      '_'.repeat(50),
      '['.repeat(2000),
    ]) {
      expect(() => parseInlineMarkdown(s)).not.toThrow();
    }
  });
});

describe('parseMarkdown', () => {
  it('parses the supported blocks', () => {
    const blocks = parseMarkdown('# T\n\n- a\n- b\n\n1. x\n2. y\n\n> q\n\n---\n\n```\ncode\n```');
    expect(blocks.map((b) => b.type)).toEqual(['heading', 'list', 'list', 'quote', 'rule', 'code']);
  });

  it('keeps plain text as one paragraph with its line breaks and blank-line gaps', () => {
    const blocks = parseMarkdown('line one\nline two\n\n\nafter');
    expect(blocks).toMatchObject([
      { type: 'paragraph', lines: ['line one', 'line two'], gap: 0 },
      { type: 'paragraph', lines: ['after'], gap: 2 },
    ]);
  });

  it('renders an oversized source as a single plain paragraph', () => {
    const big = '- x\n'.repeat(MARKDOWN_MAX_LENGTH);
    const blocks = parseMarkdown(big);
    expect(blocks).toHaveLength(1);
    expect(blocks[0].type).toBe('paragraph');
  });

  it('survives deeply nested quotes', () => {
    expect(() => parseMarkdown('> '.repeat(500) + 'x')).not.toThrow();
  });
});
