import { expect, test } from 'vitest';
import type { Activity } from '../api/activities';
import type { ExternalActivity } from '../api/externalLinks';
import type { Gift } from '../api/gifts';
import type { LifeEvent } from '../api/lifeEvents';
import type { Note } from '../api/notes';
import type { ReminderCompletion } from '../api/reminders';
import {
  buildTimelineItems,
  type ContactTimelineSources,
  fullDateFromPartial,
} from './contactTimeline';

const empty: ContactTimelineSources = {
  notes: [],
  activities: [],
  completions: [],
  lifeEvents: [],
  externalActivities: [],
  gifts: [],
};

const note = (ID: number, date: string, CreatedAt = '2020-01-01T00:00:00Z'): Note => ({
  ID,
  content: `note ${ID}`,
  date,
  CreatedAt,
  UpdatedAt: CreatedAt,
});

const activity = (ID: number, date: string, CreatedAt = '2020-01-01T00:00:00Z'): Activity => ({
  ID,
  title: `activity ${ID}`,
  date,
  CreatedAt,
  UpdatedAt: CreatedAt,
});

const lifeEvent = (id: string, overrides: Partial<LifeEvent> = {}): LifeEvent => ({
  id,
  created_at: '2019-05-05T00:00:00Z',
  updated_at: '2019-05-05T00:00:00Z',
  entity_id: 'alice-uid',
  type: 'graduated',
  ...overrides,
});

const gift = (id: string, overrides: Partial<Gift> = {}): Gift => ({
  id,
  created_at: '',
  updated_at: '',
  entity_id: 'alice-uid',
  status: 'given',
  description: `gift ${id}`,
  date: '2024-03-01T00:00:00Z',
  ...overrides,
});

// --- fullDateFromPartial ----------------------------------------------------

test('fullDateFromPartial zero-pads a full date', () => {
  expect(fullDateFromPartial({ year: 1990, month: 4, day: 3 })).toBe('1990-04-03');
});

test('fullDateFromPartial pins a yearless month/day to the given year', () => {
  expect(fullDateFromPartial({ month: 12, day: 25 }, new Date(2031, 0, 1))).toBe('2031-12-25');
});

test('fullDateFromPartial defaults the yearless case to the current year', () => {
  expect(fullDateFromPartial({ month: 1, day: 2 })).toBe(`${new Date().getFullYear()}-01-02`);
});

test('fullDateFromPartial pins a bare year to Jan 1st', () => {
  expect(fullDateFromPartial({ year: 2001 })).toBe('2001-01-01');
});

test('fullDateFromPartial treats a year+month without a day as a bare year', () => {
  expect(fullDateFromPartial({ year: 2001, month: 7 })).toBe('2001-01-01');
});

test('fullDateFromPartial returns undefined when nothing usable is set', () => {
  expect(fullDateFromPartial({})).toBeUndefined();
  expect(fullDateFromPartial({ month: 7 })).toBeUndefined();
  expect(fullDateFromPartial({ day: 7 })).toBeUndefined();
});

// --- buildTimelineItems -----------------------------------------------------

test('an empty contact has an empty timeline', () => {
  expect(buildTimelineItems(empty)).toEqual([]);
});

test('merges every source and sorts newest first', () => {
  const completion: ReminderCompletion = {
    ID: 7,
    contact_id: 1,
    message: 'Call',
    completed_at: '2024-02-01T00:00:00Z',
  };
  const external: ExternalActivity = {
    id: 'ext-1',
    created_at: '2000-01-01T00:00:00Z',
    updated_at: '',
    entity_id: 'alice-uid',
    source_system: 'immich',
    external_id: 'x',
    type: 'photo',
    occurred_at: '2024-05-01T00:00:00Z',
    provenance: 'external',
    sync_state: 'synced',
  };
  const items = buildTimelineItems({
    notes: [note(1, '2024-01-01T00:00:00Z')],
    activities: [activity(2, '2024-04-01T00:00:00Z')],
    completions: [completion],
    lifeEvents: [lifeEvent('le-1', { date: { year: 2024, month: 3, day: 15 } })],
    externalActivities: [external],
    gifts: [gift('g-1', { date: '2024-06-01T00:00:00Z' })],
  });
  expect(items.map((i) => i.type)).toEqual([
    'gift',
    'external_activity',
    'activity',
    'life_event',
    'completion',
    'note',
  ]);
  expect(items.find((i) => i.type === 'life_event')?.date).toBe('2024-03-15');
});

test('a note or activity with no date falls back to CreatedAt', () => {
  const items = buildTimelineItems({
    ...empty,
    notes: [note(1, '', '2023-01-01T00:00:00Z')],
    activities: [activity(2, '', '2022-01-01T00:00:00Z')],
  });
  expect(items.map((i) => i.date)).toEqual(['2023-01-01T00:00:00Z', '2022-01-01T00:00:00Z']);
});

test('an external activity with no occurred_at falls back to created_at', () => {
  const external = {
    id: 'ext-1',
    created_at: '2021-01-01T00:00:00Z',
    occurred_at: '',
  } as ExternalActivity;
  expect(buildTimelineItems({ ...empty, externalActivities: [external] })[0].date).toBe(
    '2021-01-01T00:00:00Z',
  );
});

test('undated life events stay off the timeline; an unusable partial date falls back to created_at', () => {
  const items = buildTimelineItems({
    ...empty,
    lifeEvents: [lifeEvent('undated'), lifeEvent('month-only', { date: { month: 6 } })],
  });
  expect(items).toHaveLength(1);
  expect(items[0].data).toMatchObject({ id: 'month-only' });
  expect(items[0].date).toBe('2019-05-05T00:00:00Z');
});

test('only dated given/received gifts are timeline events', () => {
  const items = buildTimelineItems({
    ...empty,
    gifts: [
      gift('given'),
      gift('received', { status: 'received', date: '2024-03-02T00:00:00Z' }),
      gift('idea', { status: 'idea' }),
      gift('undated', { date: undefined }),
    ],
  });
  expect(items.map((i) => (i.data as Gift).id)).toEqual(['received', 'given']);
});
