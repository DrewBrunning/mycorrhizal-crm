import { describe, expect, test } from 'vitest';
import { errorEnvelope, mockApi, setupMswServer } from '../test/mswServer';
import {
  confirmTwoFactor,
  disableTwoFactor,
  getEnabledContactFields,
  getTwoFactorStatus,
  regenerateRecoveryCodes,
  setupTwoFactor,
  updateDateFormat,
  updateEnabledContactFields,
  updateLanguage,
  updateSelfContact,
} from './users';

setupMswServer();

const fail = (code = 'BAD', msg = 'server said no') => errorEnvelope(400, code, msg);

describe('users API', () => {
  test('updateLanguage PATCHes /users/language and returns the server message', async () => {
    const calls = mockApi('patch', '/users/language', { message: 'Saved!' });
    expect(await updateLanguage('de')).toBe('Saved!');
    expect(calls[0].method).toBe('PATCH');
    expect(calls[0].url).toBe('/api/v1/users/language');
    expect(calls[0].body).toEqual({ language: 'de' });
  });

  test('updateLanguage falls back to a default message and to the default error', async () => {
    mockApi('patch', '/users/language', {});
    expect(await updateLanguage('fr')).toBe('Language updated successfully.');
    mockApi('patch', '/users/language', new Response('', { status: 500 }));
    await expect(updateLanguage('fr')).rejects.toThrow('Unable to update language.');
  });

  test('updateDateFormat PATCHes /users/date-format with date_format', async () => {
    const calls = mockApi('patch', '/users/date-format', { message: 'ok' });
    expect(await updateDateFormat('DD/MM/YYYY')).toBe('ok');
    expect(calls[0].url).toBe('/api/v1/users/date-format');
    expect(calls[0].body).toEqual({ date_format: 'DD/MM/YYYY' });
  });

  test('updateDateFormat default message and error', async () => {
    mockApi('patch', '/users/date-format', {});
    expect(await updateDateFormat('x')).toBe('Date format updated successfully.');
    mockApi('patch', '/users/date-format', fail());
    await expect(updateDateFormat('x')).rejects.toThrow('server said no');
  });

  test('getEnabledContactFields GETs and returns the list, or null when never configured', async () => {
    const calls = mockApi('get', '/users/enabled-contact-fields', {
      enabled_contact_fields: ['nickname'],
    });
    expect(await getEnabledContactFields()).toEqual(['nickname']);
    expect(calls[0].method).toBe('GET');
    mockApi('get', '/users/enabled-contact-fields', {});
    expect(await getEnabledContactFields()).toBeNull();
  });

  test('getEnabledContactFields keeps an explicitly empty list distinct from null', async () => {
    mockApi('get', '/users/enabled-contact-fields', { enabled_contact_fields: [] });
    expect(await getEnabledContactFields()).toEqual([]);
  });

  test('getEnabledContactFields error', async () => {
    mockApi('get', '/users/enabled-contact-fields', new Response('', { status: 500 }));
    await expect(getEnabledContactFields()).rejects.toThrow(
      'Unable to get enabled contact fields.',
    );
  });

  test('updateEnabledContactFields PATCHes {fields} and returns the server list or the input', async () => {
    const calls = mockApi('patch', '/users/enabled-contact-fields', {
      enabled_contact_fields: ['a', 'b'],
    });
    expect(await updateEnabledContactFields(['a'])).toEqual(['a', 'b']);
    expect(calls[0].method).toBe('PATCH');
    expect(calls[0].body).toEqual({ fields: ['a'] });
    mockApi('patch', '/users/enabled-contact-fields', {});
    expect(await updateEnabledContactFields(['z'])).toEqual(['z']);
    mockApi('patch', '/users/enabled-contact-fields', fail());
    await expect(updateEnabledContactFields([])).rejects.toThrow('server said no');
  });

  test('updateSelfContact PATCHes /users/me/self-contact with vcard_uid, null to clear', async () => {
    const calls = mockApi('patch', '/users/me/self-contact', new Response(null, { status: 204 }));
    await updateSelfContact('uid-1');
    await updateSelfContact(null);
    expect(calls[0].method).toBe('PATCH');
    expect(calls[0].url).toBe('/api/v1/users/me/self-contact');
    expect(calls[0].body).toEqual({ vcard_uid: 'uid-1' });
    expect(calls[1].body).toEqual({ vcard_uid: null });
  });

  test('updateSelfContact error', async () => {
    mockApi(
      'patch',
      '/users/me/self-contact',
      errorEnvelope(404, 'NOT_FOUND', 'Contact not found'),
    );
    await expect(updateSelfContact('nope')).rejects.toThrow('Contact not found');
  });

  test('getTwoFactorStatus coerces enabled to boolean', async () => {
    const calls = mockApi('get', '/users/2fa/status', { enabled: true });
    expect(await getTwoFactorStatus()).toEqual({ enabled: true });
    expect(calls[0].method).toBe('GET');
    expect(calls[0].url).toBe('/api/v1/users/2fa/status');
    mockApi('get', '/users/2fa/status', { enabled: 0 });
    expect(await getTwoFactorStatus()).toEqual({ enabled: false });
    mockApi('get', '/users/2fa/status', fail());
    await expect(getTwoFactorStatus()).rejects.toThrow('server said no');
  });

  test('setupTwoFactor POSTs with no body when no proof, and with the proof JSON otherwise', async () => {
    const calls = mockApi('post', '/users/2fa/setup', {
      secret: 's',
      otpauth_url: 'otpauth://x',
      extra: 1,
    });
    const out = await setupTwoFactor();
    expect(out).toEqual({ secret: 's', otpauth_url: 'otpauth://x' });
    expect(calls[0].method).toBe('POST');
    expect(calls[0].url).toBe('/api/v1/users/2fa/setup');
    expect(calls[0].body).toBeUndefined();
    await setupTwoFactor({ recovery_code: 'abc' } as never);
    expect(calls[1].body).toEqual({ recovery_code: 'abc' });
    mockApi('post', '/users/2fa/setup', fail());
    await expect(setupTwoFactor()).rejects.toThrow('server said no');
  });

  test('confirmTwoFactor POSTs {code} and returns recovery codes (empty when absent)', async () => {
    const calls = mockApi('post', '/users/2fa/confirm', { recovery_codes: ['r1', 'r2'] });
    expect(await confirmTwoFactor('123456')).toEqual({ recovery_codes: ['r1', 'r2'] });
    expect(calls[0].url).toBe('/api/v1/users/2fa/confirm');
    expect(calls[0].body).toEqual({ code: '123456' });
    mockApi('post', '/users/2fa/confirm', {});
    expect(await confirmTwoFactor('1')).toEqual({ recovery_codes: [] });
    mockApi('post', '/users/2fa/confirm', fail());
    await expect(confirmTwoFactor('1')).rejects.toThrow('server said no');
  });

  test('disableTwoFactor POSTs {code}', async () => {
    const calls = mockApi('post', '/users/2fa/disable', new Response(null, { status: 204 }));
    await disableTwoFactor('999999');
    expect(calls[0].url).toBe('/api/v1/users/2fa/disable');
    expect(calls[0].body).toEqual({ code: '999999' });
    mockApi('post', '/users/2fa/disable', new Response('', { status: 500 }));
    await expect(disableTwoFactor('1')).rejects.toThrow(
      'Unable to disable two-factor authentication.',
    );
  });

  test('regenerateRecoveryCodes wraps a string proof as {code} and passes an object proof through', async () => {
    const calls = mockApi('post', '/users/2fa/recovery-codes/regenerate', {
      recovery_codes: ['n1'],
    });
    expect(await regenerateRecoveryCodes('123456')).toEqual({ recovery_codes: ['n1'] });
    expect(calls[0].url).toBe('/api/v1/users/2fa/recovery-codes/regenerate');
    expect(calls[0].body).toEqual({ code: '123456' });
    await regenerateRecoveryCodes({ assertion: { id: 'x' } } as never);
    expect(calls[1].body).toEqual({ assertion: { id: 'x' } });
    mockApi('post', '/users/2fa/recovery-codes/regenerate', {});
    expect(await regenerateRecoveryCodes('1')).toEqual({ recovery_codes: [] });
    mockApi('post', '/users/2fa/recovery-codes/regenerate', fail());
    await expect(regenerateRecoveryCodes('1')).rejects.toThrow('server said no');
  });
});
