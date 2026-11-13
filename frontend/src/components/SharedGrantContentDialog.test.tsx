import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { ApiError } from '../api/client';
import {
  getShareGrantContent,
  type ShareGrantContent,
  type ShareGrantRecipientView,
} from '../api/shareGrants';
import { DateFormatProvider, setDateFormatPreference } from '../DateFormatProvider';
import SharedGrantContentDialog from './SharedGrantContentDialog';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

vi.mock('../api/shareGrants', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/shareGrants')>();
  return { ...actual, getShareGrantContent: vi.fn() };
});

const contentMock = vi.mocked(getShareGrantContent);

function recipientGrant(overrides: Partial<ShareGrantRecipientView> = {}): ShareGrantRecipientView {
  return {
    id: 'g1',
    from_username: 'alice',
    entity_type: 'note',
    label: 'Trip notes',
    sections: [],
    status: 'active',
    expires_at: null,
    responded_at: null,
    created_at: '2026-11-01T00:00:00Z',
    ...overrides,
  };
}

function content(overrides: Partial<ShareGrantContent>): ShareGrantContent {
  return { grant_id: 'g1', entity_type: 'note', owner_username: 'alice', ...overrides };
}

function renderDialog(grant: ShareGrantRecipientView | null, onClose = vi.fn()) {
  const view = render(
    <DateFormatProvider>
      <SharedGrantContentDialog grant={grant} onClose={onClose} />
    </DateFormatProvider>,
  );
  return { onClose, ...view };
}

beforeEach(() => {
  contentMock.mockReset();
  setDateFormatPreference('eu');
});

test('does not fetch or show anything without a grant', () => {
  renderDialog(null);
  expect(contentMock).not.toHaveBeenCalled();
  expect(screen.queryByRole('dialog')).toBeNull();
});

test('shows the title and a progress indicator while loading, then the read-only notice', async () => {
  let finish: (c: ShareGrantContent) => void = () => undefined;
  contentMock.mockReturnValue(
    new Promise((resolve) => {
      finish = resolve;
    }),
  );
  renderDialog(recipientGrant());

  expect(screen.getByText('Trip notes from alice')).toBeDefined();
  expect(screen.getByRole('progressbar', { name: 'Loading…' })).toBeDefined();
  expect(screen.queryByText(/Read-only/)).toBeNull();

  finish(content({ note: { content: 'Pack warm clothes', date: '2026-12-24' } }));
  expect(await screen.findByText('Pack warm clothes')).toBeDefined();
  expect(
    screen.getByText(/Read-only\. This is a live view of what alice currently shares/),
  ).toBeDefined();
  expect(screen.queryByRole('progressbar')).toBeNull();
});

test("a note shows its date in the user's date format and its text", async () => {
  contentMock.mockResolvedValue(
    content({ note: { content: 'Line one\nLine two', date: '2026-12-24' } }),
  );
  renderDialog(recipientGrant());

  expect(await screen.findByText('24.12.2026')).toBeDefined();
  expect(screen.getByText(/Line one/)).toBeDefined();
  expect(contentMock).toHaveBeenCalledWith('g1');
});

test('a 404 is the one "no longer available" message, whatever the reason', async () => {
  contentMock.mockRejectedValue(new ApiError('not found', 'NOT_FOUND', 404));
  renderDialog(recipientGrant());

  expect(await screen.findByText('This share is no longer available.')).toBeDefined();
  expect(screen.queryByText(/Read-only/)).toBeNull();
});

test('an offline failure shows the translated offline message, and reopening the grant fetches again', async () => {
  vi.spyOn(navigator, 'onLine', 'get').mockReturnValue(false);
  contentMock.mockRejectedValueOnce(new TypeError('Failed to fetch'));
  const { rerender } = renderDialog(recipientGrant());

  expect(
    await screen.findByText("You're offline. Check your connection and try again."),
  ).toBeDefined();

  // Close and reopen (a new grant selection): the content is re-fetched, not cached.
  contentMock.mockResolvedValueOnce(
    content({ note: { content: 'Back online', date: '2026-12-24' } }),
  );
  rerender(
    <DateFormatProvider>
      <SharedGrantContentDialog grant={null} onClose={vi.fn()} />
    </DateFormatProvider>,
  );
  rerender(
    <DateFormatProvider>
      <SharedGrantContentDialog grant={recipientGrant({ id: 'g2' })} onClose={vi.fn()} />
    </DateFormatProvider>,
  );
  expect(await screen.findByText('Back online')).toBeDefined();
  expect(contentMock).toHaveBeenLastCalledWith('g2');
});

test('a server failure shows its message', async () => {
  contentMock.mockRejectedValue(new ApiError('The server is unwell', 'INTERNAL_ERROR', 500));
  renderDialog(recipientGrant());
  expect(await screen.findByText('The server is unwell')).toBeDefined();
});

test('a response for a grant that was replaced meanwhile is ignored', async () => {
  let finishFirst: (c: ShareGrantContent) => void = () => undefined;
  contentMock.mockReturnValueOnce(
    new Promise((resolve) => {
      finishFirst = resolve;
    }),
  );
  contentMock.mockResolvedValueOnce(
    content({ note: { content: 'Second grant', date: '2026-12-24' } }),
  );
  const { rerender } = renderDialog(recipientGrant({ id: 'g1' }));
  rerender(
    <DateFormatProvider>
      <SharedGrantContentDialog grant={recipientGrant({ id: 'g2' })} onClose={vi.fn()} />
    </DateFormatProvider>,
  );
  expect(await screen.findByText('Second grant')).toBeDefined();

  await act(async () => {
    finishFirst(content({ note: { content: 'First grant (stale)', date: '2026-12-24' } }));
  });
  expect(screen.queryByText('First grant (stale)')).toBeNull();
  expect(screen.getByText('Second grant')).toBeDefined();
});

test('a contact renders its name and granted sections only', async () => {
  contentMock.mockResolvedValue(
    content({
      entity_type: 'contact',
      contact: {
        name: { full: 'Alice Johnson' },
        emails: { e1: { address: 'alice@example.com' } },
      },
    }),
  );
  renderDialog(recipientGrant({ entity_type: 'contact', label: 'Alice' }));

  expect(await screen.findByText('Alice Johnson')).toBeDefined();
  expect(screen.getByText('Emails')).toBeDefined();
  expect(screen.getByText('alice@example.com')).toBeDefined();
});

test('a contact with no details in the shared sections says so', async () => {
  contentMock.mockResolvedValue(content({ entity_type: 'contact' }));
  renderDialog(recipientGrant({ entity_type: 'contact' }));
  expect(await screen.findByText('There are no details in the shared sections.')).toBeDefined();
});

test('an activity shows its per-type fields and omits the empty ones', async () => {
  contentMock.mockResolvedValue(
    content({
      entity_type: 'activity',
      activity: {
        title: 'Hike',
        description: '',
        location: 'Alps',
        date: '2026-08-02',
        end_date: null,
        type: 'outdoor',
      },
    }),
  );
  renderDialog(recipientGrant({ entity_type: 'activity', label: 'Hike' }));

  expect(await screen.findByText('02.08.2026')).toBeDefined();
  expect(screen.getByText('Alps')).toBeDefined();
  expect(screen.getByText('outdoor')).toBeDefined();
  expect(screen.queryByText('Ends')).toBeNull();
  expect(screen.queryByText('Description')).toBeNull();
});

test('an activity with an end date shows it', async () => {
  contentMock.mockResolvedValue(
    content({
      entity_type: 'activity',
      activity: {
        title: 'Trip',
        description: 'Long weekend',
        location: '',
        date: '2026-08-02',
        end_date: '2026-08-05',
        type: '',
      },
    }),
  );
  renderDialog(recipientGrant({ entity_type: 'activity' }));
  expect(await screen.findByText('05.08.2026')).toBeDefined();
  expect(screen.getByText('Ends')).toBeDefined();
  expect(screen.getByText('Long weekend')).toBeDefined();
});

test('a circle lists its members and the truncation notice', async () => {
  contentMock.mockResolvedValue(
    content({
      entity_type: 'circle',
      circle: {
        name: 'Book club',
        description: 'Monthly',
        color: '',
        member_count: 120,
        members: [{ display_name: 'Ann' }, { display_name: 'Ben' }],
        members_truncated: true,
      },
    }),
  );
  renderDialog(recipientGrant({ entity_type: 'circle', label: 'Book club' }));

  expect(await screen.findByText('120 members')).toBeDefined();
  expect(screen.getByText('Ann')).toBeDefined();
  expect(screen.getByText('Ben')).toBeDefined();
  expect(screen.getByText('Showing the first 2 of 120 members.')).toBeDefined();
});

test('an empty circle says it has no members and shows no truncation notice', async () => {
  contentMock.mockResolvedValue(
    content({
      entity_type: 'circle',
      circle: {
        name: 'Empty',
        description: '',
        color: '',
        member_count: 0,
        members: [],
        members_truncated: false,
      },
    }),
  );
  renderDialog(recipientGrant({ entity_type: 'circle' }));
  expect(await screen.findByText('This circle has no members right now.')).toBeDefined();
  expect(screen.queryByText(/Showing the first/)).toBeNull();
});

test("a payload without the type's body renders only the notice (no crash)", async () => {
  contentMock.mockResolvedValue(content({ entity_type: 'note' }));
  renderDialog(recipientGrant());
  await waitFor(() => expect(screen.getByText(/Read-only/)).toBeDefined());
});

test('Close calls onClose', async () => {
  contentMock.mockResolvedValue(content({ note: { content: 'x', date: '2026-12-24' } }));
  const { onClose } = renderDialog(recipientGrant());
  await screen.findByText('x');
  fireEvent.click(screen.getByRole('button', { name: 'Close' }));
  expect(onClose).toHaveBeenCalled();
});
