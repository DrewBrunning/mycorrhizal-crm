import { test, expect, vi, afterEach } from 'vitest';
import { render, screen, cleanup, fireEvent, waitFor } from '@testing-library/react';
import '../i18n/config';
import NextcloudFilePickerDialog from './NextcloudFilePickerDialog';
import { WebDAVItem } from '../api/nextcloud';

afterEach(cleanup);

const rootItems: WebDAVItem[] = [{ name: 'Documents', path: '/Documents/', type: 'dir' }];
const docItems: WebDAVItem[] = [
  { name: 'contract.pdf', path: '/Documents/contract.pdf', type: 'file', size: 4096, file_id: '123' },
];

function renderDialog(overrides: Partial<React.ComponentProps<typeof NextcloudFilePickerDialog>> = {}) {
  const defaults: React.ComponentProps<typeof NextcloudFilePickerDialog> = {
    open: true,
    onClose: vi.fn(),
    onFetchDir: vi.fn().mockImplementation((path?: string) =>
      Promise.resolve(path && path.startsWith('/Documents') ? docItems : rootItems)
    ),
    onSelect: vi.fn().mockResolvedValue(undefined),
    ...overrides,
  };
  return render(<NextcloudFilePickerDialog {...defaults} />);
}

test('lists the dav root children on open', async () => {
  renderDialog();
  await waitFor(() => expect(screen.getByText('Documents')).toBeInTheDocument());
});

test('descends into a folder and back via the breadcrumb', async () => {
  const onFetchDir = vi.fn().mockImplementation((path?: string) =>
    Promise.resolve(path && path.startsWith('/Documents') ? docItems : rootItems)
  );
  renderDialog({ onFetchDir });

  await waitFor(() => expect(screen.getByText('Documents')).toBeInTheDocument());
  fireEvent.click(screen.getByText('Documents'));

  await waitFor(() => expect(screen.getByText('contract.pdf')).toBeInTheDocument());
  expect(onFetchDir).toHaveBeenLastCalledWith('/Documents/');

  // Click the root breadcrumb ("/") to go back.
  fireEvent.click(screen.getByRole('button', { name: '/' }));
  await waitFor(() => expect(onFetchDir).toHaveBeenLastCalledWith('/'));
});

test('picking a file calls onSelect with the item', async () => {
  const onSelect = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSelect });

  await waitFor(() => expect(screen.getByText('Documents')).toBeInTheDocument());
  fireEvent.click(screen.getByText('Documents'));
  await waitFor(() => expect(screen.getByText('contract.pdf')).toBeInTheDocument());

  fireEvent.click(screen.getByText('contract.pdf'));
  await waitFor(() => expect(onSelect).toHaveBeenCalledWith(docItems[0]));
});

test('an empty folder shows the empty hint', async () => {
  renderDialog({ onFetchDir: vi.fn().mockResolvedValue([]) });
  await waitFor(() => expect(screen.getByText(/empty/i)).toBeInTheDocument());
});
