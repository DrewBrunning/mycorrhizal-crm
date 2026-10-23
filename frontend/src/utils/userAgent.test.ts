import { describe, expect, test } from 'vitest';
import { describeUserAgent, formatUserAgent } from './userAgent';

describe('formatUserAgent', () => {
  test.each([
    [
      'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) HeadlessChrome/153.0.0.0 Safari/537.36',
      'Chrome 153 on Linux',
    ],
    [
      'Mozilla/5.0 (X11; Linux x86_64; rv:120.0) Gecko/20100101 Firefox/120.0',
      'Firefox 120 on Linux',
    ],
    [
      'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Edg/120.0.0.0',
      'Edge 120 on Windows',
    ],
    [
      'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15',
      'Safari 17 on macOS',
    ],
    [
      'Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36',
      'Chrome 120 on Android',
    ],
    [
      'Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1',
      'Safari 17 on iPhone',
    ],
    [
      'Mozilla/5.0 (Linux; Android 14) AppleWebKit/537.36 (KHTML, like Gecko) SamsungBrowser/23.0 Chrome/115.0.0.0 Mobile Safari/537.36',
      'Samsung Internet 23 on Android',
    ],
    [
      'Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/120.0.6099.119 Mobile/15E148 Safari/604.1',
      'Chrome 120 on iPhone',
    ],
    [
      'Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) FxiOS/121.0 Mobile/15E148 Safari/605.1.15',
      'Firefox 121 on iPhone',
    ],
    [
      'Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) EdgiOS/120.0.2210.126 Version/17.0 Mobile/15E148 Safari/604.1',
      'Edge 120 on iPad',
    ],
    ['Some Completely Unknown Agent/1.0', ''],
    ['', ''],
  ])('userAgent %s -> %s', (ua, expected) => {
    expect(formatUserAgent(ua)).toBe(expected);
  });

  test('exposes the parsed parts as well as the combined label', () => {
    expect(describeUserAgent('Mozilla/5.0 (X11; Linux x86_64) Firefox/120.0')).toEqual({
      browser: 'Firefox 120',
      platform: 'Linux',
      label: 'Firefox 120 on Linux',
    });
  });

  test('a platform-only UA still yields a usable label', () => {
    expect(formatUserAgent('Mozilla/5.0 (X11; Linux x86_64) SomeEngine/1.0')).toBe('Linux');
  });
});
