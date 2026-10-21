import { cleanup, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { CadencePolicy } from '../api/cadencePolicies';
import type { Reminder } from '../api/reminders';
import { DateFormatProvider } from '../DateFormatProvider';
import CadencePanel from './CadencePanel';
import ContactTimeline from './ContactTimeline';
import ReminderList from './ReminderList';

// Issue #1778 (web): a calendar day that travels as a 00:00Z instant (a
// reminder's remind_at, an activity/note date picked in a date input, cadence
// last_interaction) renders on that calendar day west of UTC, not the day
// before. 2026-03-11T00:00:00Z is 19:00 on 10 March in Chicago (CDT).
// Changing process.env.TZ at runtime moves Date's local getters in this
// Node/V8 build; the real-instant assertions below prove the zone took effect.

const originalTZ = process.env.TZ;

beforeEach(() => {
  process.env.TZ = 'America/Chicago';
  vi.useFakeTimers({ toFake: ['Date'] });
  // 10:00 local on 11 March.
  vi.setSystemTime(new Date('2026-03-11T15:00:00Z'));
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  process.env.TZ = originalTZ;
});

function reminder(overrides: Partial<Reminder> = {}): Reminder {
  return {
    ID: 1,
    message: 'Call back',
    by_mail: false,
    remind_at: '2026-03-11T00:00:00Z',
    recurrence: 'once',
    reoccur_from_completion: true,
    completed: false,
    email_sent: false,
    contact_id: 3,
    ...overrides,
  };
}

function renderReminders(reminders: Reminder[]) {
  render(
    <DateFormatProvider>
      <ReminderList
        reminders={reminders}
        onComplete={vi.fn().mockResolvedValue(undefined)}
        onEdit={vi.fn()}
        onDelete={vi.fn().mockResolvedValue(undefined)}
      />
    </DateFormatProvider>,
  );
}

test('a reminder due today shows today and is not overdue west of UTC', () => {
  renderReminders([reminder()]);
  const chip = screen.getByText('11.03.2026');
  expect(chip.closest('.MuiChip-colorWarning')).toBeNull();
});

test('a reminder due yesterday is overdue', () => {
  renderReminders([reminder({ remind_at: '2026-03-10T00:00:00Z' })]);
  expect(screen.getByText('10.03.2026').closest('.MuiChip-colorWarning')).not.toBeNull();
});

test('timeline note/activity dates render their calendar day; a real instant stays local', () => {
  render(
    <MemoryRouter>
      <DateFormatProvider>
        <ContactTimeline
          onEditItem={vi.fn()}
          timelineItems={[
            {
              type: 'activity',
              date: '2026-03-11T00:00:00Z',
              data: {
                ID: 1,
                title: 'Coffee',
                date: '2026-03-11T00:00:00Z',
                CreatedAt: '2026-03-11T00:00:00Z',
                UpdatedAt: '2026-03-11T00:00:00Z',
              },
            },
            {
              type: 'completion',
              // 03:30Z on 9 March is 22:30 on 8 March in Chicago.
              date: '2026-03-09T03:30:00Z',
              data: {
                ID: 2,
                contact_id: 3,
                message: 'Sent card',
                completed_at: '2026-03-09T03:30:00Z',
              },
            },
          ]}
        />
      </DateFormatProvider>
    </MemoryRouter>,
  );
  expect(screen.getByText('11.03.2026')).toBeInTheDocument();
  expect(screen.getByText('08.03.2026')).toBeInTheDocument();
});

test('cadence panel shows last interaction and next due on their API days', () => {
  const policy: CadencePolicy = {
    id: 'policy-1',
    entity_id: 'alice-uid',
    target_interval_days: 30,
    qualifying_types: ['call'],
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    health: {
      has_qualifying_interaction: true,
      overdue_by: 0,
      next_due: '2026-04-10',
      last_interaction: '2026-03-11T00:00:00Z',
    },
  };
  render(
    <DateFormatProvider>
      <CadencePanel
        policy={policy}
        loading={false}
        onAdd={vi.fn()}
        onEdit={vi.fn()}
        onDelete={vi.fn()}
      />
    </DateFormatProvider>,
  );
  expect(screen.getByText('Last interaction: 11.03.2026')).toBeInTheDocument();
  expect(screen.getByText('Next due: 10.04.2026')).toBeInTheDocument();
});
