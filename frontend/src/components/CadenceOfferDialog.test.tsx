import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import CadenceOfferDialog from './CadenceOfferDialog';

afterEach(cleanup);

const offer = { entity_id: 'alice-uid', target_interval_days: 14 };

function renderDialog(props: Partial<React.ComponentProps<typeof CadenceOfferDialog>> = {}) {
  const defaults: React.ComponentProps<typeof CadenceOfferDialog> = {
    open: true,
    contactName: 'Alice Smith',
    offer,
    onAccept: vi.fn().mockResolvedValue(undefined),
    onClose: vi.fn(),
    ...props,
  };
  return render(<CadenceOfferDialog {...defaults} />);
}

test('renders nothing when closed', () => {
  renderDialog({ open: false });
  expect(screen.queryByText('Create a cadence?')).not.toBeInTheDocument();
});

test('shows the contact name, interval, and both actions', () => {
  renderDialog();

  expect(screen.getByText('Create a cadence?')).toBeInTheDocument();
  expect(screen.getByText('Stay in touch with Alice Smith every 14 days?')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Create cadence' })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Not now' })).toBeInTheDocument();
});

test('"Create cadence" awaits onAccept with the offer, then closes', async () => {
  const onAccept = vi.fn().mockResolvedValue(undefined);
  const onClose = vi.fn();
  renderDialog({ onAccept, onClose });

  fireEvent.click(screen.getByRole('button', { name: 'Create cadence' }));

  await waitFor(() => expect(onAccept).toHaveBeenCalledWith(offer));
  await waitFor(() => expect(onClose).toHaveBeenCalledTimes(1));
});

test('"Not now" closes without making a request', () => {
  const onAccept = vi.fn().mockResolvedValue(undefined);
  const onClose = vi.fn();
  renderDialog({ onAccept, onClose });

  fireEvent.click(screen.getByRole('button', { name: 'Not now' }));

  expect(onClose).toHaveBeenCalledTimes(1);
  expect(onAccept).not.toHaveBeenCalled();
});

test('a failed accept keeps the dialog open for a retry', async () => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  const onAccept = vi.fn().mockRejectedValue(new Error('nope'));
  const onClose = vi.fn();
  renderDialog({ onAccept, onClose });

  fireEvent.click(screen.getByRole('button', { name: 'Create cadence' }));

  await waitFor(() => expect(console.error).toHaveBeenCalled());
  expect(onClose).not.toHaveBeenCalled();
  expect(screen.getByText('Create a cadence?')).toBeInTheDocument();
});
