import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { SeafileItem, SeafileLibrary } from '../api/seafile';
import SeafileFilePickerDialog from './SeafileFilePickerDialog';

afterEach(cleanup);

const libraries: SeafileLibrary[] = [{ id: 'repo-1', name: 'Personal', type: 'library' }];
const rootItems: SeafileItem[] = [
  { id: 'dir-1', name: 'Documents', type: 'dir', parent_dir: '/' },
  { id: 'f-1', name: 'readme.txt', type: 'file', size: 12, mtime: 1000, parent_dir: '/' },
];
const docItems: SeafileItem[] = [
  {
    id: 'f-2',
    name: 'contract.pdf',
    type: 'file',
    size: 4096,
    mtime: 2000,
    parent_dir: '/Documents',
  },
];

function renderDialog(
  overrides: Partial<React.ComponentProps<typeof SeafileFilePickerDialog>> = {},
) {
  const defaults: React.ComponentProps<typeof SeafileFilePickerDialog> = {
    open: true,
    onClose: vi.fn(),
    onFetchLibraries: vi.fn().mockResolvedValue(libraries),
    onFetchDir: vi
      .fn()
      .mockImplementation((_repoId: string, path: string) =>
        Promise.resolve(path === '/' ? rootItems : docItems),
      ),
    onSelect: vi.fn().mockResolvedValue(undefined),
    ...overrides,
  };
  return render(<SeafileFilePickerDialog {...defaults} />);
}

test('lists libraries first, then descends into a folder', async () => {
  renderDialog();
  await waitFor(() => expect(screen.getByText('Personal')).toBeInTheDocument());

  fireEvent.click(screen.getByText('Personal'));
  await waitFor(() => expect(screen.getByText('Documents')).toBeInTheDocument());

  fireEvent.click(screen.getByText('Documents'));
  await waitFor(() => expect(screen.getByText('contract.pdf')).toBeInTheDocument());
});

test('picking a file resolves the repo-relative path from the browse position', async () => {
  const onSelect = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSelect });

  await waitFor(() => expect(screen.getByText('Personal')).toBeInTheDocument());
  fireEvent.click(screen.getByText('Personal'));
  await waitFor(() => expect(screen.getByText('Documents')).toBeInTheDocument());

  fireEvent.click(screen.getByText('Documents'));
  await waitFor(() => expect(screen.getByText('contract.pdf')).toBeInTheDocument());

  fireEvent.click(screen.getByText('contract.pdf'));
  await waitFor(() =>
    expect(onSelect).toHaveBeenCalledWith(
      expect.objectContaining({
        repo_id: 'repo-1',
        path: '/Documents/contract.pdf',
        name: 'contract.pdf',
      }),
    ),
  );
});

test('picking a file at the library root resolves "/name"', async () => {
  const onSelect = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSelect });

  await waitFor(() => expect(screen.getByText('Personal')).toBeInTheDocument());
  fireEvent.click(screen.getByText('Personal'));
  await waitFor(() => expect(screen.getByText('readme.txt')).toBeInTheDocument());

  fireEvent.click(screen.getByText('readme.txt'));
  await waitFor(() =>
    expect(onSelect).toHaveBeenCalledWith(
      expect.objectContaining({ repo_id: 'repo-1', path: '/readme.txt' }),
    ),
  );
});
