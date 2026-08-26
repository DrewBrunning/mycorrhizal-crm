import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import {
  createLinkFieldType,
  deleteLinkFieldType,
  getLinkFieldTypes,
  type LinkFieldType,
  reorderLinkFieldTypes,
  updateLinkFieldType,
} from '../api/linkFieldTypes';
import { SnackbarProvider } from '../context/SnackbarContext';
import LinkFieldTypesSettings from './LinkFieldTypesSettings';

// This codebase's vitest setup has no auto-cleanup and no globals: true.
afterEach(cleanup);

vi.mock('../api/linkFieldTypes', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/linkFieldTypes')>();
  return {
    ...actual,
    getLinkFieldTypes: vi.fn(),
    createLinkFieldType: vi.fn(),
    updateLinkFieldType: vi.fn(),
    deleteLinkFieldType: vi.fn(),
    reorderLinkFieldTypes: vi.fn(),
  };
});

function linkFieldType(overrides: Partial<LinkFieldType> = {}): LinkFieldType {
  return {
    id: '1',
    name: 'WhatsApp',
    protocol: 'https://wa.me/{value}',
    category: 'messaging',
    is_default: true,
    position: 0,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

function renderSettings() {
  return render(
    <SnackbarProvider>
      <LinkFieldTypesSettings />
    </SnackbarProvider>,
  );
}

beforeEach(() => {
  vi.mocked(getLinkFieldTypes).mockReset();
  vi.mocked(createLinkFieldType).mockReset();
  vi.mocked(updateLinkFieldType).mockReset();
  vi.mocked(deleteLinkFieldType).mockReset();
  vi.mocked(reorderLinkFieldTypes).mockReset();
});

test('shows the empty state when there are no link field types', async () => {
  vi.mocked(getLinkFieldTypes).mockResolvedValue([]);
  renderSettings();

  await waitFor(() => expect(screen.getByText('No link field types yet.')).toBeInTheDocument());
});

test('lists a link field type under its category, flagging seeded defaults', async () => {
  vi.mocked(getLinkFieldTypes).mockResolvedValue([
    linkFieldType({ name: 'WhatsApp', category: 'messaging' }),
  ]);
  renderSettings();

  await waitFor(() => expect(screen.getByText('WhatsApp')).toBeInTheDocument());
  expect(screen.getByText('Messaging')).toBeInTheDocument();
  expect(screen.getByText('Default')).toBeInTheDocument();
});

test('a user-added type with no is_default flag shows no Default badge', async () => {
  vi.mocked(getLinkFieldTypes).mockResolvedValue([
    linkFieldType({ name: 'Custom', is_default: false }),
  ]);
  renderSettings();

  await waitFor(() => expect(screen.getByText('Custom')).toBeInTheDocument());
  expect(screen.queryByText('Default')).toBeNull();
});

test('creating a link field type posts the form', async () => {
  vi.mocked(getLinkFieldTypes).mockResolvedValue([]);
  vi.mocked(createLinkFieldType).mockResolvedValue(
    linkFieldType({ id: '9', name: 'Custom Chat', category: 'other', is_default: false }),
  );

  renderSettings();
  await waitFor(() => expect(screen.getByText('No link field types yet.')).toBeInTheDocument());

  fireEvent.click(screen.getByRole('button', { name: /add link type/i }));
  fireEvent.change(screen.getByLabelText('Name *'), { target: { value: 'Custom Chat' } });
  fireEvent.change(screen.getByLabelText('Link Template'), {
    target: { value: 'https://example.com/{value}' },
  });

  fireEvent.click(screen.getByRole('button', { name: /^save$/i }));

  await waitFor(() =>
    expect(createLinkFieldType).toHaveBeenCalledWith(
      expect.objectContaining({
        name: 'Custom Chat',
        protocol: 'https://example.com/{value}',
        category: 'messaging',
      }),
    ),
  );
});

test('editing a link field type prefills the form and saves via update, preserving is_default', async () => {
  vi.mocked(getLinkFieldTypes).mockResolvedValue([
    linkFieldType({ id: '4', name: 'Discord', protocol: '' }),
  ]);
  vi.mocked(updateLinkFieldType).mockResolvedValue(
    linkFieldType({ id: '4', name: 'Discord', protocol: 'https://discord.com/users/{value}' }),
  );

  renderSettings();
  await waitFor(() => expect(screen.getByText('Discord')).toBeInTheDocument());

  fireEvent.click(screen.getByLabelText('Edit Discord'));
  const protocolInput = screen.getByLabelText('Link Template') as HTMLInputElement;
  expect(protocolInput.value).toBe('');

  fireEvent.change(protocolInput, { target: { value: 'https://discord.com/users/{value}' } });
  fireEvent.click(screen.getByRole('button', { name: /^save$/i }));

  await waitFor(() =>
    expect(updateLinkFieldType).toHaveBeenCalledWith(
      '4',
      expect.objectContaining({ name: 'Discord', protocol: 'https://discord.com/users/{value}' }),
    ),
  );
});

test('deleting a link field type requires confirmation', async () => {
  vi.mocked(getLinkFieldTypes)
    .mockResolvedValueOnce([linkFieldType({ id: '6', name: 'To Delete' })])
    .mockResolvedValueOnce([]); // the post-delete refresh
  vi.mocked(deleteLinkFieldType).mockResolvedValue(undefined);

  renderSettings();
  await waitFor(() => expect(screen.getByText('To Delete')).toBeInTheDocument());

  fireEvent.click(screen.getByLabelText('Delete To Delete'));
  expect(deleteLinkFieldType).not.toHaveBeenCalled();
  expect(screen.getByText('Delete Link Type')).toBeInTheDocument();

  fireEvent.click(screen.getAllByRole('button', { name: /^delete$/i }).slice(-1)[0]);

  await waitFor(() => expect(deleteLinkFieldType).toHaveBeenCalledWith('6'));
  await waitFor(() => expect(screen.queryByText('To Delete')).not.toBeInTheDocument());
});

test('moving a type down persists the swapped order within its category', async () => {
  const first = linkFieldType({ id: 'a', name: 'Signal', category: 'messaging', position: 0 });
  const second = linkFieldType({ id: 'b', name: 'Telegram', category: 'messaging', position: 1 });
  vi.mocked(getLinkFieldTypes).mockResolvedValue([first, second]);
  vi.mocked(reorderLinkFieldTypes).mockResolvedValue([
    { ...second, position: 0 },
    { ...first, position: 1 },
  ]);

  renderSettings();
  await waitFor(() => expect(screen.getByText('Signal')).toBeInTheDocument());

  fireEvent.click(screen.getByLabelText('Move Signal down'));

  await waitFor(() => expect(reorderLinkFieldTypes).toHaveBeenCalledWith(['b', 'a']));
});

test('the first item in a category cannot move further up', async () => {
  vi.mocked(getLinkFieldTypes).mockResolvedValue([
    linkFieldType({ id: 'a', name: 'Signal', position: 0 }),
  ]);
  renderSettings();
  await waitFor(() => expect(screen.getByText('Signal')).toBeInTheDocument());

  expect(screen.getByLabelText('Move Signal up')).toBeDisabled();
  expect(screen.getByLabelText('Move Signal down')).toBeDisabled();
});
