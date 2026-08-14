import { test, expect, vi, afterEach } from 'vitest';
import { render, screen, cleanup, fireEvent, waitFor } from '@testing-library/react';
import '../i18n/config';
import ReviewDuplicatesDialog from './ReviewDuplicatesDialog';
import { SnackbarProvider } from '../context/SnackbarContext';

// CLAUDE.md frontend trap #1 (explicit cleanup) plus mock hygiene: the
// window.confirm spy below must not leak into later tests in this file.
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const emptyAssociationCounts = {
  notes: 0, activities: 0, reminders: 0, reminder_completions: 0, relationship_edges: 0,
  household_memberships: 0, circle_memberships: 0, tags: 0, life_events: 0,
  life_event_references: 0, field_values: 0, contact_sync_links: 0,
};

const summary = (id: number, uid: string, firstname: string, lastname: string, email = '') => ({
  id, uid, firstname, lastname, nickname: '', fn: `${firstname} ${lastname}`,
  primary_email: email, primary_phone: '', birthday: '', org: '', photo: '',
  photo_thumbnail: '', archived: false,
});

const pairsResponse = (pairs: unknown[]) => ({
  pairs,
  total: pairs.length,
  page: 1,
  limit: 100,
});

function mockFetch(handlers: {
  pairs?: () => unknown;
  onDismiss?: () => void;
  preview?: boolean;
}) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) => {
      if (url.includes('/contacts/duplicates/dismiss')) {
        handlers.onDismiss?.();
        return { ok: true, status: 200, json: async () => ({ message: 'Pair dismissed' }) };
      }
      if (url.includes('/contacts/duplicates')) {
        return { ok: true, status: 200, json: async () => (handlers.pairs ? handlers.pairs() : pairsResponse([])) };
      }
      if (handlers.preview && url.includes('/contacts/merge/preview')) {
        const body = JSON.parse(String(init?.body));
        return {
          ok: true,
          json: async () => ({
            keep_id: body.keep_id,
            merge_id: body.merge_id,
            resolution: { emails: [], phones: [], addresses: [], urls: [], impps: [], circles: [] },
            association_counts: emptyAssociationCounts,
          }),
        };
      }
      throw new Error(`unexpected fetch: ${url}`);
    })
  );
}

function renderDialog(props: Partial<React.ComponentProps<typeof ReviewDuplicatesDialog>> = {}) {
  const defaults: React.ComponentProps<typeof ReviewDuplicatesDialog> = {
    open: true,
    onClose: vi.fn(),
    ...props,
  };
  return render(
    <SnackbarProvider>
      <ReviewDuplicatesDialog {...defaults} />
    </SnackbarProvider>
  );
}

test('renders candidate pairs with their reasons and confidence', async () => {
  mockFetch({
    pairs: () =>
      pairsResponse([
        {
          a: summary(1, 'alice-uid', 'Alice', 'Adams', 'alice@example.com'),
          b: summary(2, 'bob-uid', 'Bob', 'Brown', 'ALICE@example.com'),
          reasons: ['email'],
          confidence: 0.7,
        },
      ]),
  });

  renderDialog();

  await screen.findByText('Alice Adams');
  expect(screen.getByText('Bob Brown')).toBeInTheDocument();
  expect(screen.getByText('Same email')).toBeInTheDocument();
  expect(screen.getByText('70% confidence')).toBeInTheDocument();
  expect(screen.getByText('1 candidate pairs')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Not a duplicate' })).toBeInTheDocument();
});

test('empty scan shows the no-duplicates message', async () => {
  mockFetch({});
  renderDialog();
  await screen.findByText('No duplicate candidates found.');
});

test('dismissing a pair calls the dismiss endpoint and refetches the scan', async () => {
  let remaining = [
    {
      a: summary(1, 'alice-uid', 'Alice', 'Adams', 'alice@example.com'),
      b: summary(2, 'bob-uid', 'Bob', 'Brown', 'ALICE@example.com'),
      reasons: ['email'],
      confidence: 0.7,
    },
    {
      a: summary(3, 'carol-uid', 'Carol', 'Clark'),
      b: summary(4, 'dave-uid', 'Dave', 'Dale'),
      reasons: ['name'],
      confidence: 0.5,
    },
  ];
  const onDismiss = vi.fn(() => {
    remaining = [remaining[1]];
  });
  mockFetch({ pairs: () => pairsResponse(remaining), onDismiss });

  renderDialog();
  await screen.findByText('Alice Adams');
  expect(screen.getByText('Carol Clark')).toBeInTheDocument();

  vi.spyOn(window, 'confirm').mockReturnValue(true);
  const dismissButtons = screen.getAllByRole('button', { name: 'Not a duplicate' });
  fireEvent.click(dismissButtons[0]);

  await waitFor(() => expect(onDismiss).toHaveBeenCalled());
  await waitFor(() => expect(screen.queryByText('Alice Adams')).not.toBeInTheDocument());
  expect(screen.getByText('Carol Clark')).toBeInTheDocument();
});

test('merge opens MergeContactsDialog pre-populated with the pair', async () => {
  mockFetch({
    preview: true,
    pairs: () =>
      pairsResponse([
        {
          a: summary(1, 'alice-uid', 'Alice', 'Adams'),
          b: summary(2, 'bob-uid', 'Bob', 'Brown'),
          reasons: ['name'],
          confidence: 0.5,
        },
      ]),
  });

  renderDialog();
  await screen.findByText('Alice Adams');

  fireEvent.click(screen.getByRole('button', { name: 'Merge' }));

  // MergeContactsDialog opens in pair mode with both contacts offered.
  await screen.findByLabelText('Keep Alice Adams');
  expect(screen.getByLabelText('Keep Bob Brown')).toBeInTheDocument();
});
