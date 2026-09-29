import { describe, expect, it } from 'vitest';
import { ciRequiresTools, requireToolOrSkip } from './requireCiTool';

describe('requireToolOrSkip', () => {
  it('runs when the tool is present, in CI or not', () => {
    expect(requireToolOrSkip(true, 'x', {})).toBe(true);
    expect(requireToolOrSkip(true, 'x', { CI: 'true' })).toBe(true);
  });

  it('signals skip locally when the tool is missing', () => {
    expect(requireToolOrSkip(false, 'no go', {})).toBe(false);
    expect(requireToolOrSkip(false, 'no go', { CI: '0' })).toBe(false);
  });

  it('throws in CI when the tool is missing', () => {
    expect(() => requireToolOrSkip(false, 'no go', { CI: 'true' })).toThrow(/no go/);
    expect(() => requireToolOrSkip(false, 'no go', { CI: '1' })).toThrow();
    expect(() =>
      requireToolOrSkip(false, 'no go', { MYCORRHIZAL_REQUIRE_REFERENCES: '1' }),
    ).toThrow();
  });
});

describe('ciRequiresTools', () => {
  it('treats empty/0/false as not CI', () => {
    for (const CI of ['', '0', 'false', 'FALSE', undefined]) {
      expect(ciRequiresTools({ CI })).toBe(false);
    }
  });
  it('honours the dedicated variable only when exactly 1', () => {
    expect(ciRequiresTools({ MYCORRHIZAL_REQUIRE_REFERENCES: '1' })).toBe(true);
    expect(ciRequiresTools({ MYCORRHIZAL_REQUIRE_REFERENCES: '0' })).toBe(false);
  });
  it('defaults to process.env', () => {
    expect(typeof ciRequiresTools()).toBe('boolean');
    expect(requireToolOrSkip(true, 'x')).toBe(true);
  });
});
