import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { useState } from 'react';
import { afterAll, afterEach, beforeAll, expect, test, vi } from 'vitest';
import type { Reminder } from '../api/reminders';
import { DateFormatProvider } from '../DateFormatProvider';
import i18n from '../i18n/config';
import { useTimeZone } from '../testSupport/timeZone';
import ReminderList from './ReminderList';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(cleanup);

// remind_at is a timestamp, formatted in the *local* zone (#962) -- pin the
// simulated zone so the date-chip assertions below don't depend on the test
// runner's ambient timezone. Runtime TZ mutation is ignored in worker threads
// (issue #1843), hence the Date-getter simulation.
beforeAll(() => {
  useTimeZone('UTC');
});
afterAll(() => {
  vi.restoreAllMocks();
});

function reminder(overrides: Partial<Reminder> = {}): Reminder {
  return {
    ID: 1,
    message: 'Water the plants',
    by_mail: true,
    remind_at: '2026-08-01T00:00:00Z',
    recurrence: 'once',
    reoccur_from_completion: true,
    completed: false,
    email_sent: false,
    contact_id: 3,
    ...overrides,
  };
}

function renderList(props: Partial<React.ComponentProps<typeof ReminderList>> = {}) {
  const defaults: React.ComponentProps<typeof ReminderList> = {
    reminders: [],
    onComplete: vi.fn().mockResolvedValue(undefined),
    onEdit: vi.fn(),
    onDelete: vi.fn().mockResolvedValue(undefined),
  };
  const merged = { ...defaults, ...props };
  render(
    <DateFormatProvider>
      <ReminderList {...merged} />
    </DateFormatProvider>,
  );
  return {
    onComplete: merged.onComplete,
    onEdit: merged.onEdit,
    onDelete: merged.onDelete,
  };
}

test('shows the empty state when there are no reminders', () => {
  renderList();

  expect(screen.getByText('No reminders yet')).toBeInTheDocument();
});

test('renders a reminder with its date, email and recurrence metadata', () => {
  renderList({
    reminders: [reminder({ recurrence: 'monthly' })],
  });

  expect(screen.getByText('Water the plants')).toBeInTheDocument();
  expect(screen.getByText('01.08.2026')).toBeInTheDocument();
  expect(screen.getByText('Email')).toBeInTheDocument();
  expect(screen.getByText('Monthly')).toBeInTheDocument();
  expect(screen.getByText('Flexible')).toBeInTheDocument();
});

// #196 (audit gap, N-1's sibling): revealed only on `&:hover .action-buttons`
// -- no `:focus-within` -- so keyboard focus landed on a fully transparent
// control.
test('edit/delete actions are keyboard-revealable', () => {
  renderList({ reminders: [reminder()] });
  const css = Array.from(document.querySelectorAll('style'))
    .map((s) => s.textContent || '')
    .join('\n');
  expect(css).toContain(':focus-within .action-buttons');
});

test('a one-off reminder shows neither the recurrence chip nor the flexible chip', () => {
  renderList({
    reminders: [reminder({ recurrence: 'once', reoccur_from_completion: false })],
  });

  expect(screen.queryByText('Flexible')).not.toBeInTheDocument();
  expect(screen.getByText('Email')).toBeInTheDocument();
});

test('completing a reminder calls onComplete with its id', async () => {
  const { onComplete } = renderList({
    reminders: [reminder({ ID: 5 })],
  });

  fireEvent.click(screen.getByLabelText(/^Complete\b/));

  await waitFor(() => expect(onComplete).toHaveBeenCalledWith(5));
});

test('editing a reminder passes the whole reminder through onEdit', () => {
  const theReminder = reminder({ ID: 9, message: 'Call the plumber' });
  const { onEdit } = renderList({ reminders: [theReminder] });

  fireEvent.click(screen.getByLabelText(/^Edit\b/));

  expect(onEdit).toHaveBeenCalledWith(theReminder);
});

test('deleting a reminder asks for confirmation before calling onDelete', async () => {
  const { onDelete } = renderList({ reminders: [reminder({ ID: 3 })] });

  fireEvent.click(screen.getByLabelText(/^Delete\b/));

  const dialog = screen.getByRole('dialog');
  expect(within(dialog).getByText('Delete Reminder')).toBeInTheDocument();

  fireEvent.click(within(dialog).getByRole('button', { name: /^Delete\b/ }));

  await waitFor(() => expect(onDelete).toHaveBeenCalledWith(3));
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
});

test('cancelling the delete confirmation does not call onDelete', async () => {
  const { onDelete } = renderList({ reminders: [reminder({ ID: 4 })] });

  fireEvent.click(screen.getByLabelText(/^Delete\b/));
  fireEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Cancel' }));

  expect(onDelete).not.toHaveBeenCalled();
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
});

test('an overdue reminder renders a warning-styled date chip', () => {
  renderList({ reminders: [reminder({ remind_at: '2020-01-01T00:00:00Z' })] });

  expect(screen.getByText('01.01.2020')).toBeInTheDocument();
  expect(screen.getByText('01.01.2020').closest('.MuiChip-colorWarning')).not.toBeNull();
});

// #2673: each reminder's buttons name the reminder.
test('complete, edit and delete buttons name the reminder', () => {
  renderList({ reminders: [reminder(), reminder({ ID: 2, message: 'Send Chen the book' })] });
  expect(
    screen.getByRole('button', { name: 'Complete reminder Send Chen the book' }),
  ).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Edit Water the plants' })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Delete Send Chen the book' })).toBeInTheDocument();
});

// #2915: an occasion reminder's title is a localized template, not the
// English "Occasion — " prefix stored in the row. The backend now stores
// "label: name"; older rows still carry the prefix, which must not double up.
test('renders a new-format occasion reminder with the localized title (#2915)', () => {
  renderList({
    reminders: [
      reminder({ message: 'Card for Cory: Cory Noaddr', occasion_obligation_id: 'ob-1' }),
    ],
  });

  expect(screen.getByText('Occasion — Card for Cory: Cory Noaddr')).toBeInTheDocument();
});

test('renders a legacy occasion reminder without doubling the stored prefix (#2915)', () => {
  renderList({
    reminders: [
      reminder({
        message: 'Occasion — Card for Cory: Cory Noaddr',
        occasion_obligation_id: 'ob-1',
      }),
    ],
  });

  expect(screen.getByText('Occasion — Card for Cory: Cory Noaddr')).toBeInTheDocument();
  expect(
    screen.queryByText('Occasion — Occasion — Card for Cory: Cory Noaddr'),
  ).not.toBeInTheDocument();
});

test('localizes the occasion title in the active language (#2915)', async () => {
  await i18n.changeLanguage('de');
  try {
    renderList({
      reminders: [
        reminder({ message: 'Karte für Cory: Cory Noaddr', occasion_obligation_id: 'ob-1' }),
      ],
    });

    expect(screen.getByText('Anlass — Karte für Cory: Cory Noaddr')).toBeInTheDocument();
  } finally {
    await i18n.changeLanguage('en');
  }
});

test('leaves a reminder with no occasion link unchanged (#2915)', () => {
  renderList({ reminders: [reminder({ message: 'Occasion — just a message' })] });

  expect(screen.getByText('Occasion — just a message')).toBeInTheDocument();
});

// #2918 / #2920: deleting a reminder unmounts the row that held focus.
function Harness({ initial }: { initial: Reminder[] }) {
  const [reminders, setReminders] = useState(initial);
  return (
    <DateFormatProvider>
      <ReminderList
        reminders={reminders}
        onComplete={vi.fn()}
        onEdit={vi.fn()}
        onDelete={async (id) => setReminders((r) => r.filter((x) => x.ID !== id))}
      />
    </DateFormatProvider>
  );
}

test('deleting a reminder moves focus to the next row, then the empty state', async () => {
  render(
    <Harness
      initial={[reminder({ ID: 1, message: 'First' }), reminder({ ID: 2, message: 'Second' })]}
    />,
  );
  const confirmDelete = async () => {
    const dialog = await screen.findByRole('dialog');
    fireEvent.click(within(dialog).getByRole('button', { name: /^Delete\b/ }));
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
  };

  fireEvent.click(screen.getByLabelText(/^Delete First/));
  await confirmDelete();
  await waitFor(() => expect(document.activeElement).toBe(screen.getByLabelText(/^Delete Second/)));

  fireEvent.click(screen.getByLabelText(/^Delete Second/));
  await confirmDelete();
  await waitFor(() =>
    expect(document.activeElement).toBe(screen.getByText('No reminders yet').parentElement),
  );
});
