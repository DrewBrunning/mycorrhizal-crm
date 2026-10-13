import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import RecordConflictDialog from './RecordConflictDialog';

afterEach(cleanup);

test('renders nothing when closed', () => {
  render(<RecordConflictDialog open={false} onKeepEditing={vi.fn()} onReload={vi.fn()} />);
  expect(screen.queryByText('Changed elsewhere')).not.toBeInTheDocument();
});

test('explains the conflict and offers both actions', () => {
  render(<RecordConflictDialog open onKeepEditing={vi.fn()} onReload={vi.fn()} />);
  expect(screen.getByText('Changed elsewhere')).toBeInTheDocument();
  expect(screen.getByText(/never merged automatically/)).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Keep editing' })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Load latest version' })).toBeInTheDocument();
});

test('the buttons call their handlers', () => {
  const onKeepEditing = vi.fn();
  const onReload = vi.fn();
  render(<RecordConflictDialog open onKeepEditing={onKeepEditing} onReload={onReload} />);
  fireEvent.click(screen.getByRole('button', { name: 'Keep editing' }));
  expect(onKeepEditing).toHaveBeenCalledTimes(1);
  fireEvent.click(screen.getByRole('button', { name: 'Load latest version' }));
  expect(onReload).toHaveBeenCalledTimes(1);
});
