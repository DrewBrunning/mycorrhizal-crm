import { expect, test } from 'vitest';
import { secretRequiredForOriginChange, urlOrigin } from './urlOrigin';

test('urlOrigin normalizes case and default ports and rejects garbage', () => {
  expect(urlOrigin('https://Svc.Example:443/a')).toBe('https://svc.example');
  expect(urlOrigin(' http://svc.example:80 ')).toBe('http://svc.example');
  expect(urlOrigin('not a url')).toBeNull();
});

test('secretRequiredForOriginChange mirrors the backend rule', () => {
  const stored = 'https://svc.example';
  // Nothing stored: never required.
  expect(secretRequiredForOriginChange(false, stored, 'https://elsewhere.example')).toBe(false);
  expect(secretRequiredForOriginChange(undefined, stored, 'https://elsewhere.example')).toBe(false);
  // Same origin (path/case/default-port changes): optional.
  expect(secretRequiredForOriginChange(true, stored, 'https://SVC.example:443/sub')).toBe(false);
  // Different host, scheme, or port: required.
  expect(secretRequiredForOriginChange(true, stored, 'https://elsewhere.example')).toBe(true);
  expect(secretRequiredForOriginChange(true, stored, 'http://svc.example')).toBe(true);
  expect(secretRequiredForOriginChange(true, stored, 'https://svc.example:8443')).toBe(true);
  // Unparseable typed URL is the form validator's problem; unparseable stored fails closed.
  expect(secretRequiredForOriginChange(true, stored, 'nope')).toBe(false);
  expect(secretRequiredForOriginChange(true, '', 'https://svc.example')).toBe(true);
});
