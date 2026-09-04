import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import ConfirmDiscardDialog from './ConfirmDiscardDialog';

afterEach(cleanup);

test('renders nothing when closed', () => {
  render(<ConfirmDiscardDialog open={false} onKeepEditing={vi.fn()} onDiscard={vi.fn()} />);
  expect(screen.queryByText('Discard unsaved changes?')).not.toBeInTheDocument();
});

test('shows the title, message, and both actions when open', () => {
  render(<ConfirmDiscardDialog open onKeepEditing={vi.fn()} onDiscard={vi.fn()} />);

  expect(screen.getByText('Discard unsaved changes?')).toBeInTheDocument();
  expect(
    screen.getByText('You have unsaved changes that will be lost if you continue.'),
  ).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Keep editing' })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Discard' })).toBeInTheDocument();
});

test('"Keep editing" calls onKeepEditing', () => {
  const onKeepEditing = vi.fn();
  render(<ConfirmDiscardDialog open onKeepEditing={onKeepEditing} onDiscard={vi.fn()} />);

  fireEvent.click(screen.getByRole('button', { name: 'Keep editing' }));

  expect(onKeepEditing).toHaveBeenCalledTimes(1);
});

test('"Discard" calls onDiscard', () => {
  const onDiscard = vi.fn();
  render(<ConfirmDiscardDialog open onKeepEditing={vi.fn()} onDiscard={onDiscard} />);

  fireEvent.click(screen.getByRole('button', { name: 'Discard' }));

  expect(onDiscard).toHaveBeenCalledTimes(1);
});

// AppDialog blocks a stray backdrop click (it shakes instead of closing) --
// Escape maps to "keep editing", the same safe default a plain dismiss
// should have.
test('Escape calls onKeepEditing, not onDiscard', () => {
  const onKeepEditing = vi.fn();
  const onDiscard = vi.fn();
  render(<ConfirmDiscardDialog open onKeepEditing={onKeepEditing} onDiscard={onDiscard} />);

  fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape', code: 'Escape' });

  expect(onKeepEditing).toHaveBeenCalledTimes(1);
  expect(onDiscard).not.toHaveBeenCalled();
});
