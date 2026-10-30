import { describe, expect, test } from 'vitest';
import { errorEnvelope, mockApi, setupMswServer } from '../test/mswServer';
import {
  type ContactFieldVisibility,
  getContactFieldVisibility,
  putContactFieldVisibility,
  resetContactFieldVisibility,
} from './contactFieldVisibility';
import { getHideEmptyFields, updateHideEmptyFields } from './users';

setupMswServer();

const visibility: ContactFieldVisibility = {
  overrides: { emails: 'hide' },
  hide_empty: 'inherit',
  defaults: { fields: { emails: true }, hide_empty: false },
  resolved: { fields: { emails: false }, hide_empty: false },
};

describe('contact field visibility API', () => {
  test('getContactFieldVisibility GETs the contact sub-resource', async () => {
    const calls = mockApi('get', '/contacts/7/field-visibility', visibility);
    expect(await getContactFieldVisibility(7)).toEqual(visibility);
    expect(calls[0].method).toBe('GET');
    expect(calls[0].url).toBe('/api/v1/contacts/7/field-visibility');
  });

  test('putContactFieldVisibility PUTs the complete override set and sends no If-Match', async () => {
    const calls = mockApi('put', '/contacts/7/field-visibility', visibility);
    expect(
      await putContactFieldVisibility(7, { overrides: { emails: 'hide' }, hide_empty: 'inherit' }),
    ).toEqual(visibility);
    expect(calls[0].method).toBe('PUT');
    expect(calls[0].body).toEqual({ overrides: { emails: 'hide' }, hide_empty: 'inherit' });
    expect(JSON.stringify(calls[0].headers ?? {})).not.toMatch(/if-match/i);
  });

  test('resetContactFieldVisibility DELETEs the overrides and returns the new decision', async () => {
    const calls = mockApi('delete', '/contacts/7/field-visibility', visibility);
    expect(await resetContactFieldVisibility(7)).toEqual(visibility);
    expect(calls[0].method).toBe('DELETE');
  });

  test('a 4xx/5xx rejects with the server message', async () => {
    mockApi(
      'get',
      '/contacts/7/field-visibility',
      errorEnvelope(404, 'NOT_FOUND', 'Contact not found'),
    );
    await expect(getContactFieldVisibility(7)).rejects.toThrow('Contact not found');
    mockApi('put', '/contacts/7/field-visibility', errorEnvelope(400, 'INVALID_INPUT', 'bad key'));
    await expect(putContactFieldVisibility(7, {})).rejects.toThrow('bad key');
    mockApi('delete', '/contacts/7/field-visibility', errorEnvelope(404, 'NOT_FOUND', 'gone'));
    await expect(resetContactFieldVisibility(7)).rejects.toThrow('gone');
  });
});

describe('account hide-empty default', () => {
  test('getHideEmptyFields reads hide_empty_fields from the enabled-contact-fields GET', async () => {
    mockApi('get', '/users/enabled-contact-fields', {
      enabled_contact_fields: null,
      hide_empty_fields: true,
    });
    expect(await getHideEmptyFields()).toBe(true);
    mockApi('get', '/users/enabled-contact-fields', { enabled_contact_fields: null });
    expect(await getHideEmptyFields()).toBe(false);
  });

  test('updateHideEmptyFields PATCHes /users/hide-empty-fields', async () => {
    const calls = mockApi('patch', '/users/hide-empty-fields', { hide_empty_fields: true });
    expect(await updateHideEmptyFields(true)).toBe(true);
    expect(calls[0].method).toBe('PATCH');
    expect(calls[0].body).toEqual({ hide_empty_fields: true });
  });

  test('both reject with the default message when the server gives none', async () => {
    mockApi('get', '/users/enabled-contact-fields', new Response('', { status: 500 }));
    await expect(getHideEmptyFields()).rejects.toThrow('Unable to get the hide-empty setting.');
    mockApi('patch', '/users/hide-empty-fields', new Response('', { status: 500 }));
    await expect(updateHideEmptyFields(false)).rejects.toThrow(
      'Unable to update the hide-empty setting.',
    );
  });
});
