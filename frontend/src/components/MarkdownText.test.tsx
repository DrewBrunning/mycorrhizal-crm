import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import '../i18n/config';
import MarkdownText from './MarkdownText';

afterEach(cleanup);

describe('MarkdownText', () => {
  it('renders plain text with line breaks preserved (pre-wrap) and no extra elements', () => {
    const { container } = render(<MarkdownText text={'one\ntwo'} />);
    const root = container.firstElementChild as HTMLElement;
    expect(getComputedStyle(root).whiteSpace).toBe('pre-wrap');
    expect(root.textContent).toBe('one\ntwo');
    expect(root.querySelectorAll('a,ul,ol,h3,strong,em').length).toBe(0);
  });

  it('renders the Markdown subset', () => {
    const { container } = render(
      <MarkdownText text={'# Head\n\n- a\n- b\n\n**bold** and *it* and `c`\n\n> quote\n\n---'} />,
    );
    expect(container.querySelector('h3')?.textContent).toBe('Head');
    expect(container.querySelectorAll('ul li')).toHaveLength(2);
    expect(container.querySelector('strong')?.textContent).toBe('bold');
    expect(container.querySelector('em')?.textContent).toBe('it');
    expect(container.querySelector('code')?.textContent).toBe('c');
    expect(container.querySelector('blockquote')).not.toBeNull();
    expect(container.querySelector('hr')).not.toBeNull();
  });

  it('renders raw HTML payloads inert (as text)', () => {
    const payload =
      '<img src=x onerror="alert(1)"><script>alert(1)</script><a href="javascript:alert(1)">x</a>';
    const { container } = render(<MarkdownText text={payload} />);
    expect(container.querySelector('img,script,a')).toBeNull();
    expect(container.textContent).toBe(payload);
  });

  it.each([
    '[x](javascript:alert(1))',
    '[x](JAVASCRIPT:alert(1))',
    '[x](java\tscript:alert(1))',
    '[x](data:text/html;base64,PHNjcmlwdD4=)',
    '[x](vbscript:x)',
    '![x](javascript:alert(1))',
    '![x](https://e.com/p.png)',
    '[x](https://e.com "t" onclick="alert(1)")',
    '**[x](javascript:alert(1))**',
    '> [x](javascript:alert(1))',
    '- [x](javascript:alert(1))',
  ])('never emits an unsafe link or element for %j', (src) => {
    const { container } = render(<MarkdownText text={src} />);
    expect(container.querySelector('img,script,iframe')).toBeNull();
    for (const a of Array.from(container.querySelectorAll('a'))) {
      expect(new URL(a.getAttribute('href') ?? '').hostname).toBe('e.com');
    }
    expect(container.innerHTML).not.toMatch(/\son\w+=/i);
  });

  it('renders safe external links with rel, target and an accessible name', () => {
    render(<MarkdownText text="see [the docs](https://example.com/d)" />);
    const link = screen.getByRole('link', { name: /the docs/ });
    expect(link.getAttribute('href')).toBe('https://example.com/d');
    expect(link.getAttribute('target')).toBe('_blank');
    expect(link.getAttribute('rel')).toContain('noopener');
    expect(link.getAttribute('rel')).toContain('noreferrer');
    expect(link.textContent).toContain('opens in a new tab');
    expect(link.querySelector('svg')?.getAttribute('aria-hidden')).toBe('true');
  });

  it('gives an empty-label link its URL as the name', () => {
    render(<MarkdownText text="[](https://example.com)" />);
    expect(
      screen.getByRole('link', { name: 'https://example.com (opens in a new tab)' }),
    ).toBeTruthy();
  });

  it('tolerates malformed and deeply nested Markdown', () => {
    const { container } = render(
      <MarkdownText text={'**a *b `c [d](\n> > > > > > x\n```\nunclosed'} />,
    );
    expect(container.querySelector('script')).toBeNull();
  });
});
