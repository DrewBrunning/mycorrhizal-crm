import { test, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import '../i18n/config';
import GiftDialog from './GiftDialog';
import { Gift } from '../api/gifts';
import { DateFormatProvider } from '../DateFormatProvider';

afterEach(cleanup);

const gift: Gift = {
  id: 'gift-1',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  entity_id: 'alice-uid',
  description: 'The espresso machine',
  status: 'purchased',
  occasion: 'birthday',
  date: '2026-08-03T12:00:00Z',
  value_cents: 12000,
  currency: 'EUR',
};

const lifeEvents = [
  { id: 'evt-1', created_at: '', updated_at: '', entity_id: 'alice-uid', type: 'married' },
];

const activities = [
  { ID: 7, title: 'Coffee catch-up', date: '2026-08-03T12:00:00Z', CreatedAt: '', UpdatedAt: '' },
];

function renderDialog(props: Partial<React.ComponentProps<typeof GiftDialog>> = {}) {
  const defaults: React.ComponentProps<typeof GiftDialog> = {
    open: true,
    onClose: vi.fn(),
    onSave: vi.fn().mockResolvedValue(undefined),
    gift,
    lifeEvents,
    activities,
    ...props,
  };
  return render(
    <DateFormatProvider>
      <GiftDialog {...defaults} />
    </DateFormatProvider>
  );
}

test('prefills the editable fields from the gift', () => {
  renderDialog();
  expect(screen.getByLabelText('What the gift is *')).toHaveValue('The espresso machine');
  expect(screen.getByLabelText('Occasion (optional)')).toHaveValue('birthday');
  expect(screen.getByLabelText('Date')).toHaveValue('2026-08-03');
  expect(screen.getByLabelText('Amount (optional)')).toHaveValue(120);
  expect(screen.getByLabelText('Currency')).toHaveValue('EUR');
});

test('save submits the converted value/currency pair and ISO date', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave });

  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await vi.waitFor(() => expect(onSave).toHaveBeenCalled());
  const submitted = onSave.mock.calls[0][0];
  expect(submitted).toMatchObject({
    status: 'purchased',
    description: 'The espresso machine',
    occasion: 'birthday',
    value_cents: 12000,
    currency: 'EUR',
  });
  expect(submitted.date.startsWith('2026-08-03')).toBe(true);
});

test('a new gift without a value sends no currency and a null date', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ gift: null, onSave });

  fireEvent.change(screen.getByLabelText('What the gift is *'), {
    target: { value: 'A new idea' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await vi.waitFor(() => expect(onSave).toHaveBeenCalled());
  const submitted = onSave.mock.calls[0][0];
  expect(submitted.status).toBe('idea');
  expect(submitted.value_cents).toBeUndefined();
  expect(submitted.currency).toBeUndefined();
  expect(submitted.date).toBeNull();
});

test('an empty description is rejected without calling onSave', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ gift: null, onSave });

  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await vi.waitFor(() =>
    expect(screen.getByText(/Describe the gift or idea/i)).toBeInTheDocument()
  );
  expect(onSave).not.toHaveBeenCalled();
});

test('an amount without a currency is rejected', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ gift: null, onSave });

  fireEvent.change(screen.getByLabelText('What the gift is *'), {
    target: { value: 'A watch' },
  });
  fireEvent.change(screen.getByLabelText('Amount (optional)'), { target: { value: '50' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await vi.waitFor(() =>
    expect(screen.getByText(/currency is required/i)).toBeInTheDocument()
  );
  expect(onSave).not.toHaveBeenCalled();
});

// --- T35: URL + notes fields ---

test('prefills the URL and notes fields from the gift', () => {
  renderDialog({
    gift: { ...gift, url: 'https://shop.example.com/machine', notes: 'Check the voltage' },
  });

  expect(screen.getByLabelText('Link (optional)')).toHaveValue('https://shop.example.com/machine');
  expect(screen.getByLabelText('Notes (optional)')).toHaveValue('Check the voltage');
});

test('save submits a trimmed URL and notes', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ gift: null, onSave });

  fireEvent.change(screen.getByLabelText('What the gift is *'), {
    target: { value: 'The ceramic mug' },
  });
  fireEvent.change(screen.getByLabelText('Link (optional)'), {
    target: { value: '  https://shop.example.com/mug  ' },
  });
  fireEvent.change(screen.getByLabelText('Notes (optional)'), {
    target: { value: '  Size medium  ' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await vi.waitFor(() => expect(onSave).toHaveBeenCalled());
  expect(onSave.mock.calls[0][0]).toMatchObject({
    url: 'https://shop.example.com/mug',
    notes: 'Size medium',
  });
});

test('empty URL and notes are sent as undefined, not empty strings', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ gift: null, onSave });

  fireEvent.change(screen.getByLabelText('What the gift is *'), {
    target: { value: 'A new idea' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await vi.waitFor(() => expect(onSave).toHaveBeenCalled());
  const submitted = onSave.mock.calls[0][0];
  expect(submitted.url).toBeUndefined();
  expect(submitted.notes).toBeUndefined();
});

test('a scheme-less URL is normalized to https so it renders as a link', async () => {
  // Without this the value saves happily and then shows as dead text, because
  // GiftList only links an absolute URI.
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ gift: null, onSave });

  fireEvent.change(screen.getByLabelText('What the gift is *'), {
    target: { value: 'The ceramic mug' },
  });
  fireEvent.change(screen.getByLabelText('Link (optional)'), {
    target: { value: 'shop.example.com/mug' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await vi.waitFor(() => expect(onSave).toHaveBeenCalled());
  expect(onSave.mock.calls[0][0].url).toBe('https://shop.example.com/mug');
});

test('an unsafe-scheme URL is rejected client-side without calling onSave', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ gift: null, onSave });

  fireEvent.change(screen.getByLabelText('What the gift is *'), {
    target: { value: 'A watch' },
  });
  fireEvent.change(screen.getByLabelText('Link (optional)'), {
    target: { value: 'javascript:alert(1)' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await vi.waitFor(() => expect(screen.getByText(/unsafe scheme/i)).toBeInTheDocument());
  expect(onSave).not.toHaveBeenCalled();
});

test('a non-http scheme the old validator accepted is rejected client-side (T41)', async () => {
  // safeurl accepted mailto:; the web-link fields are http(s)-only now, and
  // the dialog must catch it with a readable message rather than a 400.
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ gift: null, onSave });

  fireEvent.change(screen.getByLabelText('What the gift is *'), {
    target: { value: 'A watch' },
  });
  fireEvent.change(screen.getByLabelText('Link (optional)'), {
    target: { value: 'mailto:a@b.com' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await vi.waitFor(() => expect(screen.getByText(/unsafe scheme/i)).toBeInTheDocument());
  expect(onSave).not.toHaveBeenCalled();
});

test('the status selector lets a brand-new gift start as given', async () => {
  // T35's full-form path: the dialog opened with no gift behind it must be
  // able to record something already given, without an idea round trip.
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ gift: null, onSave });

  fireEvent.change(screen.getByLabelText('What the gift is *'), {
    target: { value: 'The scarf' },
  });
  fireEvent.mouseDown(screen.getByLabelText('Status'));
  fireEvent.click(screen.getByRole('option', { name: 'Given' }));
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await vi.waitFor(() => expect(onSave).toHaveBeenCalled());
  expect(onSave.mock.calls[0][0]).toMatchObject({ status: 'given', description: 'The scarf' });
});

test('the life event and activity selectors offer the links', () => {
  renderDialog();
  fireEvent.mouseDown(screen.getByLabelText(/Relates to a life event/i));
  expect(screen.getByText('Married')).toBeInTheDocument();

  fireEvent.mouseDown(screen.getByLabelText(/Relates to an interaction/i));
  expect(screen.getByText(/Coffee catch-up/)).toBeInTheDocument();
});
