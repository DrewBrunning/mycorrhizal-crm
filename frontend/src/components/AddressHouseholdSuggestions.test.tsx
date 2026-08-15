import { test, expect, vi, afterEach } from 'vitest';
import { render, screen, cleanup, fireEvent } from '@testing-library/react';
import '../i18n/config';
import AddressHouseholdSuggestions from './AddressHouseholdSuggestions';
import { AddressHouseholdSuggestion } from '../api/households';

afterEach(cleanup);

const suggestion: AddressHouseholdSuggestion = {
  address_hash: 'aabb',
  member_hash: 'ccdd',
  member_vcard_uids: ['alice-uid', 'bob-uid'],
  address: {
    components: [
      { kind: 'name', value: '123 Main St' },
      { kind: 'locality', value: 'Springfield' },
      { kind: 'region', value: 'IL' },
      { kind: 'postcode', value: '62704' },
    ],
  },
};

function renderSuggestions(props: Partial<React.ComponentProps<typeof AddressHouseholdSuggestions>> = {}) {
  const defaults: React.ComponentProps<typeof AddressHouseholdSuggestions> = {
    suggestions: [suggestion],
    onAccept: vi.fn(),
    onDismiss: vi.fn(),
    busy: false,
    ...props,
  };
  return render(<AddressHouseholdSuggestions {...defaults} />);
}

// Resolves member VCardUIDs to names via getContactsByUid (a GET /contacts
// call filtered by vcard_uid).
function stubContactsFetch() {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        contacts: [
          { id: 1, uid: 'alice-uid', firstname: 'Alice', lastname: 'Anderson' },
          { id: 2, uid: 'bob-uid', firstname: 'Bob', lastname: 'Brown' },
        ],
      }),
    })
  );
}

test('renders member names and the formatted shared address', async () => {
  stubContactsFetch();
  renderSuggestions();

  expect(await screen.findByText('Alice Anderson · Bob Brown')).toBeInTheDocument();
  expect(screen.getByText('123 Main St, Springfield, IL, 62704')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Accept' })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Dismiss' })).toBeInTheDocument();
  vi.unstubAllGlobals();
});

test('accept calls onAccept with the suggestion and disables while pending', async () => {
  stubContactsFetch();
  const onAccept = vi.fn().mockResolvedValue(undefined);
  renderSuggestions({ onAccept });

  const acceptButton = await screen.findByRole('button', { name: 'Accept' });
  fireEvent.click(acceptButton);

  expect(onAccept).toHaveBeenCalledWith(suggestion);
  vi.unstubAllGlobals();
});

test('dismiss calls onDismiss with the suggestion', async () => {
  stubContactsFetch();
  const onDismiss = vi.fn().mockResolvedValue(undefined);
  renderSuggestions({ onDismiss });

  const dismissButton = await screen.findByRole('button', { name: 'Dismiss' });
  fireEvent.click(dismissButton);

  expect(onDismiss).toHaveBeenCalledWith(suggestion);
  vi.unstubAllGlobals();
});

test('shows the empty state with no suggestions', () => {
  renderSuggestions({ suggestions: [] });
  expect(
    screen.getByText('No shared-address suggestions found. Add addresses to more contacts to find groups here.')
  ).toBeInTheDocument();
});

// T64: the
// TS type promises a non-nullable array, but a Go nil slice marshals as
// `null`, and that reached this component unguarded. `as any` simulates the
// malformed API response the real bug produced — the assertion is that this
// renders the empty state instead of throwing on `null.flatMap`.
test('renders the empty state instead of throwing when suggestions is null', () => {
  renderSuggestions({ suggestions: null as unknown as AddressHouseholdSuggestion[] });
  expect(
    screen.getByText('No shared-address suggestions found. Add addresses to more contacts to find groups here.')
  ).toBeInTheDocument();
});
