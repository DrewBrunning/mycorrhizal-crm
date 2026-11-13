import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { ApiError } from '../api/client';
import { type ShareGrantOwnerView, updateShareGrant } from '../api/shareGrants';
import EditShareGrantDialog from './EditShareGrantDialog';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(cleanup);

vi.mock('../api/shareGrants', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/shareGrants')>();
  return { ...actual, updateShareGrant: vi.fn() };
});

// The real field parses typed text in the user's date-format preference; the
// dialog only depends on its YYYY-MM-DD value/onChange contract.
vi.mock('./DateField', () => ({
  default: ({
    label,
    value,
    onChange,
  }: {
    label: string;
    value: string;
    onChange: (v: string) => void;
  }) => <input aria-label={label} value={value} onChange={(e) => onChange(e.target.value)} />,
}));

const updateMock = vi.mocked(updateShareGrant);

function grant(overrides: Partial<ShareGrantOwnerView> = {}): ShareGrantOwnerView {
  return {
    id: 'g1',
    to_user_id: 2,
    to_username: 'bob',
    entity_type: 'contact',
    entity_id: 'uid-1',
    label: 'Alice Johnson',
    sections: ['emails', 'phones', 'notes'],
    reconfirm_sections: [],
    status: 'active',
    expires_at: '2026-12-31T23:59:59Z',
    responded_at: null,
    revoked_at: null,
    revoked_reason: null,
    member_count: null,
    created_at: '2026-11-01T00:00:00Z',
    updated_at: '2026-11-01T00:00:00Z',
    revision: 4,
    ...overrides,
  };
}

function renderDialog(g: ShareGrantOwnerView | null, handlers = {}) {
  const props = { onClose: vi.fn(), onSaved: vi.fn(), onStale: vi.fn(), ...handlers };
  render(<EditShareGrantDialog grant={g} {...props} />);
  return props;
}

const saveButton = () => screen.getByRole('button', { name: 'Save' }) as HTMLButtonElement;

beforeEach(() => {
  updateMock.mockReset();
});

test('renders nothing without a grant', () => {
  renderDialog(null);
  expect(screen.queryByRole('dialog')).toBeNull();
});

test("lists only the grant's current sections (narrow-only), all kept, with Save disabled until something changes", () => {
  renderDialog(grant());

  expect(screen.getByText(/To share more, create a new share/)).toBeDefined();
  for (const label of ['Emails', 'Phones', 'Notes']) {
    expect((screen.getByRole('checkbox', { name: label }) as HTMLInputElement).checked).toBe(true);
  }
  expect(screen.getAllByRole('checkbox')).toHaveLength(3);
  expect(saveButton().disabled).toBe(true);
});

test('removing a section sends only the remaining sections with the grant revision', async () => {
  const saved = grant({ sections: ['emails', 'notes'], revision: 5 });
  updateMock.mockResolvedValue(saved);
  const { onSaved } = renderDialog(grant());

  fireEvent.click(screen.getByRole('checkbox', { name: 'Phones' }));
  expect(saveButton().disabled).toBe(false);
  fireEvent.click(saveButton());

  await waitFor(() => expect(onSaved).toHaveBeenCalledWith(saved));
  expect(updateMock).toHaveBeenCalledWith('g1', { sections: ['emails', 'notes'] }, 4);
});

test('re-ticking a removed section restores the unchanged state', () => {
  renderDialog(grant());
  const phones = screen.getByRole('checkbox', { name: 'Phones' });
  fireEvent.click(phones);
  fireEvent.click(phones);
  expect(saveButton().disabled).toBe(true);
});

test('removing every section cannot be saved', () => {
  renderDialog(grant());
  for (const label of ['Emails', 'Phones', 'Notes']) {
    fireEvent.click(screen.getByRole('checkbox', { name: label }));
  }
  expect(saveButton().disabled).toBe(true);
});

test('changing only the expiry sends only expires_at at the end of that day; clearing it sends null', async () => {
  updateMock.mockResolvedValue(grant());
  renderDialog(grant());

  const expiry = screen.getByLabelText('Expires on (optional)') as HTMLInputElement;
  expect(expiry.value).toBe('2026-12-31');
  fireEvent.change(expiry, { target: { value: '2027-03-05' } });
  fireEvent.click(saveButton());
  await waitFor(() => expect(updateMock).toHaveBeenCalledTimes(1));
  const [, patch, revision] = updateMock.mock.calls[0];
  expect(Object.keys(patch)).toEqual(['expires_at']);
  const sent = new Date(patch.expires_at as string);
  expect([sent.getFullYear(), sent.getMonth(), sent.getDate(), sent.getHours()]).toEqual([
    2027, 2, 5, 23,
  ]);
  expect(revision).toBe(4);

  cleanup();
  updateMock.mockClear();
  renderDialog(grant());
  fireEvent.change(screen.getByLabelText('Expires on (optional)'), { target: { value: '' } });
  fireEvent.click(saveButton());
  await waitFor(() => expect(updateMock).toHaveBeenCalledWith('g1', { expires_at: null }, 4));
});

test('a non-contact grant has no section list and can still change its expiry', async () => {
  updateMock.mockResolvedValue(grant({ entity_type: 'circle', sections: [] }));
  renderDialog(grant({ entity_type: 'circle', sections: [], expires_at: null }));

  expect(screen.queryAllByRole('checkbox')).toHaveLength(0);
  expect(saveButton().disabled).toBe(true);
  fireEvent.change(screen.getByLabelText('Expires on (optional)'), {
    target: { value: '2027-01-01' },
  });
  expect(saveButton().disabled).toBe(false);
});

test('a stale write (412) shows the conflict message and asks the page to reload', async () => {
  updateMock.mockRejectedValue(new ApiError('stale', 'PRECONDITION_FAILED', 412));
  const { onStale, onSaved } = renderDialog(grant());

  fireEvent.click(screen.getByRole('checkbox', { name: 'Notes' }));
  fireEvent.click(saveButton());

  expect(
    await screen.findByText(
      'This share was changed elsewhere. The list has been reloaded; try again.',
    ),
  ).toBeDefined();
  expect(onStale).toHaveBeenCalledTimes(1);
  expect(onSaved).not.toHaveBeenCalled();
  expect(saveButton().disabled).toBe(false);
});

test('a reason-coded refusal is shown translated and does not reload the list', async () => {
  updateMock.mockRejectedValue(
    new ApiError(
      'raw server text',
      'CONFLICT',
      409,
      undefined,
      undefined,
      'share_grant_widening_refused',
    ),
  );
  const { onStale } = renderDialog(grant());

  fireEvent.click(screen.getByRole('checkbox', { name: 'Notes' }));
  fireEvent.click(saveButton());

  const alert = await screen.findByRole('alert');
  expect(alert.textContent).not.toBe('');
  expect(onStale).not.toHaveBeenCalled();
});

test('Save is disabled while the request is in flight, and Cancel closes', async () => {
  let finish: (g: ShareGrantOwnerView) => void = () => undefined;
  updateMock.mockReturnValue(
    new Promise((resolve) => {
      finish = resolve;
    }),
  );
  const { onClose, onSaved } = renderDialog(grant());

  fireEvent.click(screen.getByRole('checkbox', { name: 'Notes' }));
  fireEvent.click(saveButton());
  await waitFor(() => expect(saveButton().disabled).toBe(true));
  expect((screen.getByRole('button', { name: 'Cancel' }) as HTMLButtonElement).disabled).toBe(true);

  finish(grant({ sections: ['emails', 'phones'] }));
  await waitFor(() => expect(onSaved).toHaveBeenCalled());

  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
  expect(onClose).toHaveBeenCalled();
});
