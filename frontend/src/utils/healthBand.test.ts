import { describe, expect, test } from 'vitest';
import { targetStatusBand } from './healthBand';

describe('targetStatusBand (issue #1797)', () => {
  test.each([
    ['on_track', 'moss'],
    ['drifting', 'chanterelle'],
    ['behind', 'russula'],
  ] as const)('maps %s to %s', (status, band) => {
    expect(targetStatusBand(status)).toBe(band);
  });

  test('maps an unknown status to undefined', () => {
    expect(targetStatusBand('something_else')).toBeUndefined();
  });

  test('maps undefined to undefined', () => {
    expect(targetStatusBand(undefined)).toBeUndefined();
  });
});
