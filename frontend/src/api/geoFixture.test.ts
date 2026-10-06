import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, test } from 'vitest';

import { formatGeoUri, parseCoordinateInput, parseGeoUri } from './map';

// The shared geo: URI parity fixture (also read by the Go and Android tests).
// Go is the validation authority: every input it accepts (valid=true) must be
// accepted here, and every formatter output must parse.
interface ParseCase {
  input: string;
  valid: boolean;
  lat?: number;
  lon?: number;
}
interface FormatCase {
  lat: number;
  lon: number;
  expected: string;
}
const fixture = JSON.parse(
  readFileSync(resolve(__dirname, '../../../testdata/geo-uri-fixtures.json'), 'utf8'),
) as { parse: ParseCase[]; format: FormatCase[]; coordinateInput: ParseCase[] };

describe('geo URI fixture parity', () => {
  test('fixture is non-empty', () => {
    expect(fixture.parse.length).toBeGreaterThan(0);
    expect(fixture.format.length).toBeGreaterThan(0);
  });

  // valid=false cases are Go-only: clients may be more lenient.
  test.each(fixture.parse.filter((c) => c.valid))('parseGeoUri accepts $input', (c) => {
    const got = parseGeoUri(c.input);
    expect(got).not.toBeNull();
    expect(got?.lat).toBeCloseTo(c.lat as number, 12);
    expect(got?.lng).toBeCloseTo(c.lon as number, 12);
  });

  test.each(fixture.format)('formatGeoUri($lat, $lon)', (c) => {
    const got = formatGeoUri(c.lat, c.lon);
    expect(got).toBe(c.expected);
    expect(got.slice(4)).not.toMatch(/[eE]/);
    expect(parseGeoUri(got)).not.toBeNull();
  });

  test.each(fixture.coordinateInput)('parseCoordinateInput $input', (c) => {
    const got = parseCoordinateInput(c.input);
    if (!c.valid) {
      expect(got).toBeNull();
      return;
    }
    expect(got?.lat).toBeCloseTo(c.lat as number, 12);
    expect(got?.lng).toBeCloseTo(c.lon as number, 12);
  });
});
