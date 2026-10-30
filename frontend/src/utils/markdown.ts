// Small, deliberate CommonMark subset for notes and activity descriptions (#2005).
//
// This is a parser, not a renderer: it returns a plain data tree that
// components/MarkdownText.tsx turns into React elements. Nothing here produces
// HTML, so raw HTML in the source is just text and there is no
// dangerouslySetInnerHTML anywhere. The stored source is never modified.
//
// Supported blocks: ATX headings, bullet/ordered lists (flat), blockquotes,
// horizontal rules, fenced code, paragraphs. Supported inline: `code`,
// **bold**, *italic* / _italic_, [text](url), backslash escapes. Images are NOT
// rendered (their source stays visible as text); images go through the image
// feature. Bare URLs are not auto-linked, so existing plain text is unchanged.

export type Inline =
  | { kind: 'text'; text: string }
  | { kind: 'code'; text: string }
  | { kind: 'strong'; children: Inline[] }
  | { kind: 'em'; children: Inline[] }
  | { kind: 'link'; href: string; external: boolean; children: Inline[] };

export type Block = { gap: number } & (
  | { type: 'paragraph'; lines: string[] }
  | { type: 'heading'; level: number; text: string }
  | { type: 'list'; ordered: boolean; start: number; items: string[] }
  | { type: 'quote'; blocks: Block[] }
  | { type: 'rule' }
  | { type: 'code'; text: string }
);

// Past this size the source is rendered as a single plain paragraph: it bounds
// the worst-case cost of inline scanning on pathological input.
export const MARKDOWN_MAX_LENGTH = 50_000;
const MAX_INLINE_DEPTH = 6;
const MAX_QUOTE_DEPTH = 4;

const ALLOWED_PROTOCOLS = new Set(['http:', 'https:', 'mailto:', 'tel:']);

/**
 * Returns a safe, normalized href, or null when the URL is not allowed.
 * Only http(s), mailto and tel pass; javascript:, data:, vbscript:, relative and
 * protocol-relative URLs are rejected. Browsers ignore ASCII control characters
 * and whitespace inside a scheme (`java\tscript:`), so they are stripped before
 * the check and the stripped form is what gets emitted.
 */
export function sanitizeUrl(raw: string): string | null {
  const cleaned = Array.from(raw)
    .filter((c) => {
      const code = c.codePointAt(0)!;
      return !(
        code <= 0x20 ||
        (code >= 0x7f && code <= 0x9f) ||
        code === 0x2028 ||
        code === 0x2029
      );
    })
    .join('');
  if (!cleaned) return null;
  let parsed: URL;
  try {
    parsed = new URL(cleaned);
  } catch {
    return null;
  }
  if (!ALLOWED_PROTOCOLS.has(parsed.protocol)) return null;
  if ((parsed.protocol === 'http:' || parsed.protocol === 'https:') && !parsed.hostname) {
    return null;
  }
  return cleaned;
}

const ESCAPABLE = '\\`*_{}[]()#+-.!>~|<';

function parseInline(src: string, depth = 0): Inline[] {
  const out: Inline[] = [];
  let buf = '';
  const flush = () => {
    if (buf) {
      out.push({ kind: 'text', text: buf });
      buf = '';
    }
  };
  let i = 0;
  while (i < src.length) {
    const ch = src[i];

    if (ch === '\\' && i + 1 < src.length && ESCAPABLE.includes(src[i + 1])) {
      buf += src[i + 1];
      i += 2;
      continue;
    }

    if (ch === '`') {
      const end = src.indexOf('`', i + 1);
      if (end > i + 1) {
        flush();
        out.push({ kind: 'code', text: src.slice(i + 1, end) });
        i = end + 1;
        continue;
      }
    }

    // Image syntax is kept as literal text (no <img> from notes).
    if (ch === '!' && src[i + 1] === '[') {
      const m = /^!\[[^\]]*\]\([^)]*\)/.exec(src.slice(i));
      if (m) {
        buf += m[0];
        i += m[0].length;
        continue;
      }
    }

    if (ch === '[' && depth < MAX_INLINE_DEPTH) {
      const close = src.indexOf(']', i + 1);
      if (close > i && src[close + 1] === '(') {
        const paren = src.indexOf(')', close + 2);
        if (paren > close) {
          const label = src.slice(i + 1, close);
          // Drop an optional "title" after the URL.
          const target =
            src
              .slice(close + 2, paren)
              .trim()
              .split(/\s+/)[0] ?? '';
          const href = sanitizeUrl(target);
          if (href) {
            flush();
            out.push({
              kind: 'link',
              href,
              external: /^https?:/i.test(href),
              children: label ? parseInline(label, depth + 1) : [{ kind: 'text', text: href }],
            });
            i = paren + 1;
            continue;
          }
        }
      }
    }

    if ((ch === '*' || ch === '_') && depth < MAX_INLINE_DEPTH) {
      const double = src[i + 1] === ch;
      const marker = double ? ch + ch : ch;
      const intraword = ch === '_' && i > 0 && /[A-Za-z0-9]/.test(src[i - 1]);
      const start = i + marker.length;
      const end = intraword ? -1 : src.indexOf(marker, start);
      if (end > start && !/^\s/.test(src[start]) && !/\s/.test(src[end - 1])) {
        flush();
        const children = parseInline(src.slice(start, end), depth + 1);
        out.push({ kind: double ? 'strong' : 'em', children });
        i = end + marker.length;
        continue;
      }
    }

    buf += ch;
    i += 1;
  }
  flush();
  return out;
}

export function parseInlineMarkdown(src: string): Inline[] {
  return parseInline(src);
}

const FENCE = /^ {0,3}(```|~~~)/;
const HEADING = /^ {0,3}(#{1,6})[ \t]+(.*)$/;
const RULE_CANDIDATE = /^ {0,3}[-*_][-*_ \t]*$/;
const RULE = {
  test(line: string): boolean {
    if (!RULE_CANDIDATE.test(line)) return false;
    const marks = line.replace(/[ \t]/g, '');
    return marks.length >= 3 && [...marks].every((c) => c === marks[0]);
  },
};
const BULLET = /^ {0,3}[-*+][ \t]+(.*)$/;
const ORDERED = /^ {0,3}(\d{1,9})[.)][ \t]+(.*)$/;
const QUOTE = /^ {0,3}>[ \t]?(.*)$/;

function startsBlock(line: string): boolean {
  return (
    FENCE.test(line) ||
    HEADING.test(line) ||
    RULE.test(line) ||
    BULLET.test(line) ||
    ORDERED.test(line) ||
    QUOTE.test(line)
  );
}

function parseBlocks(lines: string[], quoteDepth: number): Block[] {
  const blocks: Block[] = [];
  let blanks = 0;
  const push = (b: Omit<Block, 'gap'> & Record<string, unknown>) => {
    blocks.push({ ...b, gap: blocks.length === 0 ? 0 : Math.max(blanks, 1) } as Block);
    blanks = 0;
  };

  let i = 0;
  while (i < lines.length) {
    const line = lines[i];
    if (line.trim() === '') {
      blanks += 1;
      i += 1;
      continue;
    }

    const fence = FENCE.exec(line);
    if (fence) {
      const body: string[] = [];
      i += 1;
      while (i < lines.length && !lines[i].trimStart().startsWith(fence[1])) {
        body.push(lines[i]);
        i += 1;
      }
      i += 1; // closing fence (or EOF: an unclosed fence runs to the end)
      push({ type: 'code', text: body.join('\n') });
      continue;
    }

    const heading = HEADING.exec(line);
    if (heading) {
      push({
        type: 'heading',
        level: heading[1].length,
        text: heading[2].replace(/[ \t]+#+[ \t]*$/, '').trim(),
      });
      i += 1;
      continue;
    }

    if (RULE.test(line)) {
      push({ type: 'rule' });
      i += 1;
      continue;
    }

    if (QUOTE.test(line)) {
      const inner: string[] = [];
      while (i < lines.length) {
        const quote = QUOTE.exec(lines[i]);
        if (!quote) break;
        inner.push(quote[1]);
        i += 1;
      }
      push({
        type: 'quote',
        blocks:
          quoteDepth >= MAX_QUOTE_DEPTH
            ? [{ type: 'paragraph', lines: inner, gap: 0 }]
            : parseBlocks(inner, quoteDepth + 1),
      });
      continue;
    }

    const bullet = BULLET.exec(line);
    const ordered = ORDERED.exec(line);
    if (bullet || ordered) {
      const isOrdered = !bullet;
      const items: string[] = [];
      const start = ordered ? Number(ordered[1]) : 1;
      while (i < lines.length) {
        const m = isOrdered ? ORDERED.exec(lines[i]) : BULLET.exec(lines[i]);
        // A rule like "* * *" matches BULLET; it ends the list.
        if (!m || RULE.test(lines[i])) break;
        let item = isOrdered ? m[2] : m[1];
        i += 1;
        // Indented continuation lines belong to the item.
        while (
          i < lines.length &&
          /^ {2,}\S/.test(lines[i]) &&
          !startsBlock(lines[i].trimStart())
        ) {
          item += '\n' + lines[i].trim();
          i += 1;
        }
        items.push(item);
      }
      push({ type: 'list', ordered: isOrdered, start, items });
      continue;
    }

    const para: string[] = [line];
    i += 1;
    while (i < lines.length && lines[i].trim() !== '' && !startsBlock(lines[i])) {
      para.push(lines[i]);
      i += 1;
    }
    push({ type: 'paragraph', lines: para });
  }
  return blocks;
}

export function parseMarkdown(source: string): Block[] {
  const normalized = source.replace(/\r\n?/g, '\n');
  if (normalized.length > MARKDOWN_MAX_LENGTH) {
    return [{ type: 'paragraph', lines: [normalized], gap: 0 }];
  }
  return parseBlocks(normalized.split('\n'), 0);
}
