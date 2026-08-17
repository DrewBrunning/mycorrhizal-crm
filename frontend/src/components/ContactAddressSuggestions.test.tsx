import { test, expect, vi, afterEach } from 'vitest';
import { render, screen, cleanup, waitFor } from '@testing-library/react';
import '../i18n/config';
import ContactAddressSuggestions from './ContactAddressSuggestions';
import { ContactAddressSuggestion } from '../api/dataSuggestions';

afterEach(cleanup);

const suggestion: ContactAddressSuggestion = {
  contact_vcard_uid: 'alice-uid',
  contact_name: 'Alice Anderson',
  source_kind: 'relationship',
  source_id: 'bob-uid',
  source_name: 'Bob Brown',
  relation_type: 'spouse_of',
  address: {
    components: [
      { kind: 'name', value: '123 Main St' },
      { kind: 'locality', value: 'Springfield' },
      { kind: 'region', value: 'IL' },
      { kind: 'postcode', value: '62704' },
    ],
  },
  address_key: '123 main st|springfield|il|62704',
};

function stubFetch(initial: ContactAddressSuggestion[], applyCalls: { method: string; url: string }[] = []) {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockImplementation((url: string, init?: RequestInit) => {
      const method = init?.method || 'GET';
      if (method === 'POST' && String(url).includes('/address-suggestions/apply')) {
        applyCalls.push({ method, url: String(url) });
        return Promise.resolve({ ok: true, json: async () => ({ message: 'ok' }) });
      }
      // The scan returns the full list on the first load, then (post-apply)
      // the caller reloads the inbox; keep returning the same list so the
      // test can assert the apply call rather than list mutation.
      return Promise.resolve({
        ok: true,
        json: async () => ({ suggestions: initial, total: initial.length }),
      });
    })
  );
}

test('renders contact, formatted address, and the reason for the suggestion', async () => {
  stubFetch([suggestion]);
  render(<ContactAddressSuggestions loadKey={0} />);

  expect(await screen.findByText('Alice Anderson')).toBeInTheDocument();
  expect(screen.getByText('123 Main St, Springfield, IL, 62704')).toBeInTheDocument();
  expect(screen.getByText('Shared with Bob Brown (Spouse)')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Apply' })).toBeInTheDocument();
  vi.unstubAllGlobals();
});

test('renders a household reason', async () => {
  const householdSuggestion: ContactAddressSuggestion = {
    ...suggestion,
    source_kind: 'household',
    source_id: 'home-id',
    source_name: 'The Anderson Home',
    relation_type: undefined,
  };
  stubFetch([householdSuggestion]);
  render(<ContactAddressSuggestions loadKey={0} />);

  expect(await screen.findByText('Household: The Anderson Home')).toBeInTheDocument();
  vi.unstubAllGlobals();
});

// The backend serializes ContactAddressSuggestion.Address with
// `components,omitempty`, so a source address with no non-empty flat fields
// (e.g. an all-empty address row on a related contact) arrives as
// `address: {}` — no `components`, no `full`. formatSuggestionAddress must
// not throw on that shape, or the whole Data page lands in the ErrorBoundary
// as soon as it opens.
test('does not crash when a suggestion address has no components', async () => {
  const bare: ContactAddressSuggestion = {
    ...suggestion,
    address: {},
    address_key: 'a|b|c',
  };
  stubFetch([bare]);
  render(<ContactAddressSuggestions loadKey={0} />);

  expect(await screen.findByText('Alice Anderson')).toBeInTheDocument();
  vi.unstubAllGlobals();
});

test('shows the empty state when there are no suggestions', async () => {
  stubFetch([]);
  render(<ContactAddressSuggestions loadKey={0} />);

  expect(await screen.findByText('No address suggestions right now.')).toBeInTheDocument();
  vi.unstubAllGlobals();
});

test('apply posts the suggestion identity to the apply endpoint', async () => {
  const applyCalls: { method: string; url: string }[] = [];
  stubFetch([suggestion], applyCalls);
  render(<ContactAddressSuggestions loadKey={0} />);

  const applyButton = await screen.findByRole('button', { name: 'Apply' });
  applyButton.click();

  await waitFor(() => {
    expect(applyCalls).toHaveLength(1);
  });
  expect(applyCalls[0]).toEqual({
    method: 'POST',
    url: expect.stringContaining('/contacts/address-suggestions/apply'),
  });
  vi.unstubAllGlobals();
});
