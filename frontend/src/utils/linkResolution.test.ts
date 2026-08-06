import { test, expect } from 'vitest';
import {
  buildTelLink,
  buildSmsLink,
  buildMailtoLink,
  isSafeUrlString,
  isHttpUrlString,
  looksLikeAbsoluteUri,
  resolveOnlineServiceLink,
  formatAddressLine,
  buildAddressLink,
} from './linkResolution';
import { LinkFieldType } from '../api/linkFieldTypes';

test('buildTelLink/buildSmsLink/buildMailtoLink build the expected scheme URIs', () => {
  expect(buildTelLink('+15551234567')).toBe('tel:+15551234567');
  expect(buildSmsLink('+15551234567')).toBe('sms:+15551234567');
  expect(buildMailtoLink('alice@example.com')).toBe('mailto:alice@example.com');
});

test('isSafeUrlString accepts http(s) and empty values', () => {
  expect(isSafeUrlString('https://example.com')).toBe(true);
  expect(isSafeUrlString('http://example.com')).toBe(true);
  expect(isSafeUrlString('')).toBe(true);
  expect(isSafeUrlString('geo:39.78,-89.65')).toBe(true);
  expect(isSafeUrlString('tel:+15551234567')).toBe(true);
});

test('isSafeUrlString rejects javascript:/data:/vbscript:/file: schemes, mirroring the backend validator', () => {
  expect(isSafeUrlString('javascript:alert(1)')).toBe(false);
  expect(isSafeUrlString('data:text/html,<script>alert(1)</script>')).toBe(false);
  expect(isSafeUrlString('vbscript:msgbox(1)')).toBe(false);
  expect(isSafeUrlString('file:///etc/passwd')).toBe(false);
  // Case-insensitive and whitespace-tolerant, like the backend validator.
  expect(isSafeUrlString('JavaScript:alert(1)')).toBe(false);
  expect(isSafeUrlString('java\tscript:alert(1)')).toBe(false);
});

test('isSafeUrlString rejects an unsafe scheme even after {value} substitution -- not just the raw template', () => {
  const template = 'javascript:alert(1)//{value}';
  const substituted = template.replace('{value}', encodeURIComponent('anything'));
  expect(isSafeUrlString(substituted)).toBe(false);
});

// isHttpUrlString is the client mirror of backend validateHTTPURL (T41). This
// table intentionally mirrors backend/middleware/validation_test.go's
// TestValidateStruct_HTTPURL exactly — the two implementations disagreeing is
// the actual risk the ticket exists to close. Keep them in step.
test('isHttpUrlString accepts http(s) and empty values', () => {
  expect(isHttpUrlString('')).toBe(true);
  expect(isHttpUrlString('   ')).toBe(true);
  expect(isHttpUrlString('https://example.com/path?q=1')).toBe(true);
  expect(isHttpUrlString('http://example.com')).toBe(true);
  expect(isHttpUrlString('http://immich:2283')).toBe(true);
});

test('isHttpUrlString rejects everything that is not http(s), including no scheme at all', () => {
  expect(isHttpUrlString('example.com')).toBe(false);
  expect(isHttpUrlString('example.com:8080/x')).toBe(false);
  expect(isHttpUrlString('//example.com/x')).toBe(false);
  expect(isHttpUrlString('mailto:a@b.com')).toBe(false);
  expect(isHttpUrlString('javascript:alert(1)')).toBe(false);
  expect(isHttpUrlString('JavaScript:alert(1)')).toBe(false);
  expect(isHttpUrlString('java\tscript:alert(1)')).toBe(false);
  expect(isHttpUrlString('data:text/html,<script>')).toBe(false);
  expect(isHttpUrlString('vbscript:msgbox(1)')).toBe(false);
  expect(isHttpUrlString('file:///etc/passwd')).toBe(false);
  expect(isHttpUrlString('blob:https://example.com/abc-123')).toBe(false);
  expect(isHttpUrlString('intent://scan/#Intent;scheme=zebra;end')).toBe(false);
  expect(isHttpUrlString('ms-msdt:/id:PCWAlert;S:RunProgram;X:cmd')).toBe(false);
});

// The two guards are intentionally different: Card.Links/IMPP are legitimately
// non-http (xmpp:, sip:, mailto:), web-link fields are not. A scheme the
// blocklist lets through must not leak into the allowlist.
test('isSafeUrlString accepts schemes that isHttpUrlString rejects', () => {
  expect(isSafeUrlString('mailto:a@b.com')).toBe(true);
  expect(isHttpUrlString('mailto:a@b.com')).toBe(false);
  expect(isSafeUrlString('intent://scan/#Intent;scheme=zebra;end')).toBe(true);
  expect(isHttpUrlString('intent://scan/#Intent;scheme=zebra;end')).toBe(false);
});

test('looksLikeAbsoluteUri distinguishes a real URI from a bare handle', () => {
  expect(looksLikeAbsoluteUri('https://example.com')).toBe(true);
  expect(looksLikeAbsoluteUri('xmpp:user@example.com')).toBe(true);
  expect(looksLikeAbsoluteUri('geo:1,2')).toBe(true);
  expect(looksLikeAbsoluteUri('some_handle')).toBe(false);
  expect(looksLikeAbsoluteUri('')).toBe(false);
});

const whatsapp: LinkFieldType = {
  id: '1', name: 'WhatsApp', protocol: 'https://wa.me/{value}', category: 'messaging',
  is_default: true, position: 0, created_at: '', updated_at: '',
};
const noTemplate: LinkFieldType = {
  id: '2', name: 'Discord', protocol: '', category: 'messaging',
  is_default: true, position: 1, created_at: '', updated_at: '',
};

test('resolveOnlineServiceLink prefers a full URI when present', () => {
  const link = resolveOnlineServiceLink({ uri: 'https://example.com/alice', service: 'WhatsApp', user: '15551234567' }, [whatsapp]);
  expect(link).toBe('https://example.com/alice');
});

test('resolveOnlineServiceLink rejects an unsafe full URI rather than falling back to the registry', () => {
  const link = resolveOnlineServiceLink({ uri: 'javascript:alert(1)', service: 'WhatsApp', user: '15551234567' }, [whatsapp]);
  expect(link).toBeNull();
});

test('resolveOnlineServiceLink matches Service against the registry case-insensitively and substitutes {value}', () => {
  const link = resolveOnlineServiceLink({ service: 'whatsapp', user: '15551234567' }, [whatsapp]);
  expect(link).toBe('https://wa.me/15551234567');
});

test('resolveOnlineServiceLink URL-encodes the substituted handle', () => {
  const link = resolveOnlineServiceLink({ service: 'WhatsApp', user: 'a b/c' }, [whatsapp]);
  expect(link).toBe('https://wa.me/a%20b%2Fc');
});

test('resolveOnlineServiceLink returns null for a matched type with an empty protocol (e.g. Discord)', () => {
  const link = resolveOnlineServiceLink({ service: 'Discord', user: 'alice#1234' }, [noTemplate]);
  expect(link).toBeNull();
});

test('resolveOnlineServiceLink returns null when nothing matches and there is no URI', () => {
  const link = resolveOnlineServiceLink({ service: 'SomeUnknownService', user: 'alice' }, [whatsapp]);
  expect(link).toBeNull();
});

test('resolveOnlineServiceLink returns null when the handle is missing even if the service matches', () => {
  const link = resolveOnlineServiceLink({ service: 'WhatsApp' }, [whatsapp]);
  expect(link).toBeNull();
});

// A bare handle with no scheme (e.g. imported straight into the URI slot)
// passes isSafeUrlString -- it has no dangerous scheme, but it also isn't a
// URI at all. Without the looksLikeAbsoluteUri guard this would render as a
// bogus relative/same-origin href instead of falling through to "not
// tappable".
test('resolveOnlineServiceLink rejects a URI value that has no scheme at all', () => {
  const link = resolveOnlineServiceLink({ uri: 'alice@example.com', service: 'WhatsApp', user: '15551234567' }, [whatsapp]);
  expect(link).toBeNull();
});

test('resolveOnlineServiceLink rejects a protocol-relative URI value', () => {
  const link = resolveOnlineServiceLink({ uri: '//evil.example.com/x' }, [whatsapp]);
  expect(link).toBeNull();
});

// A template referencing {value} more than once (e.g. a query-string
// callback param) must have every occurrence substituted, not just the
// first -- .replace(string, ...) only replaces the first match.
test('resolveOnlineServiceLink substitutes every occurrence of {value} in the template', () => {
  const repeated: LinkFieldType = {
    id: '3', name: 'Repeated', protocol: 'https://x.example.com/{value}?ref={value}', category: 'other',
    is_default: false, position: 0, created_at: '', updated_at: '',
  };
  const link = resolveOnlineServiceLink({ service: 'Repeated', user: 'alice' }, [repeated]);
  expect(link).toBe('https://x.example.com/alice?ref=alice');
});

test('formatAddressLine joins non-empty parts and skips blanks', () => {
  expect(formatAddressLine({ street: '', city: 'Springfield', region: 'IL', postal: '', country: '' })).toBe('Springfield, IL');
  expect(formatAddressLine({ street: '', city: '', region: '', postal: '', country: '' })).toBe('');
});

test('buildAddressLink prefers a geo: URI when coordinates are present', () => {
  const href = buildAddressLink({ type: '', street: '', city: 'Springfield', region: '', postal: '', country: '', coordinates: 'geo:39.78,-89.65' });
  expect(href).toBe('geo:39.78,-89.65');
});

test('buildAddressLink rejects an unsafe coordinates value', () => {
  const href = buildAddressLink({ type: '', street: '', city: '', region: '', postal: '', country: '', coordinates: 'javascript:alert(1)' });
  expect(href).toBeNull();
});

test('buildAddressLink falls back to a Google Maps search built from the formatted address', () => {
  const href = buildAddressLink({ type: '', street: '', city: 'Springfield', region: 'IL', postal: '', country: '' });
  expect(href).toBe('https://maps.google.com/?q=' + encodeURIComponent('Springfield, IL'));
});

test('buildAddressLink returns null when there is nothing to link to', () => {
  const href = buildAddressLink({ type: '', street: '', city: '', region: '', postal: '', country: '' });
  expect(href).toBeNull();
});
