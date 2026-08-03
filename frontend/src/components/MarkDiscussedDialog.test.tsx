import { test, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import '../i18n/config';
import MarkDiscussedDialog from './MarkDiscussedDialog';
import { ConversationAgenda } from '../api/conversationAgenda';
import { Activity } from '../api/activities';
import { DateFormatProvider } from '../DateFormatProvider';

afterEach(cleanup);

const item: ConversationAgenda = {
  id: 'agenda-1',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  entity_id: 'alice-uid',
  content: 'Ask about her mother\u2019s surgery',
};

const activities: Activity[] = [
  { ID: 7, title: 'Coffee catch-up', date: '2026-08-03T10:00:00Z', CreatedAt: '2026-08-03T10:00:00Z', UpdatedAt: '2026-08-03T10:00:00Z' },
  { ID: 9, title: 'Phone call', date: '2026-07-20T10:00:00Z', CreatedAt: '2026-07-20T10:00:00Z', UpdatedAt: '2026-07-20T10:00:00Z' },
];

function renderDialog(props: Partial<React.ComponentProps<typeof MarkDiscussedDialog>> = {}) {
  const defaults: React.ComponentProps<typeof MarkDiscussedDialog> = {
    open: true,
    onClose: vi.fn(),
    onConfirm: vi.fn().mockResolvedValue(undefined),
    item,
    activities: activities,
    ...props,
  };
  return render(
    <DateFormatProvider>
      <MarkDiscussedDialog {...defaults} />
    </DateFormatProvider>
  );
}

test('shows the item content and offers the optional activity dropdown', () => {
  renderDialog();
  expect(screen.getByText('Ask about her mother’s surgery')).toBeInTheDocument();
  expect(screen.getByLabelText('Recorded in (optional)')).toBeInTheDocument();
});

test('confirm with no activity selected calls onConfirm(undefined)', async () => {
  const onConfirm = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onConfirm });

  fireEvent.click(screen.getByRole('button', { name: 'Mark as discussed' }));

  await vi.waitFor(() => expect(onConfirm).toHaveBeenCalledWith(undefined));
  expect(onConfirm.mock.calls[0][0]).toBeUndefined();
});

test('confirm with a selected activity calls onConfirm with its id', async () => {
  const onConfirm = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onConfirm });

  fireEvent.mouseDown(screen.getByLabelText('Recorded in (optional)'));
  fireEvent.click(screen.getByRole('option', { name: /Coffee catch-up/ }));
  fireEvent.click(screen.getByRole('button', { name: 'Mark as discussed' }));

  await vi.waitFor(() => expect(onConfirm).toHaveBeenCalledWith(7));
});

test('hides the activity dropdown when there are no activities (marking stays valid)', () => {
  renderDialog({ activities: [] });
  expect(screen.queryByLabelText('Recorded in (optional)')).not.toBeInTheDocument();
});
