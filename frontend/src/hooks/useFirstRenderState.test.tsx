// Issue #1662 stryker-disable audit: the initial value of every hook's
// `loading` state (and useTimeline's idle state) is observable. The first
// render commits before any mount effect can run, and consumers read it --
// NetworkPage, for instance, uses useCircles' loading=true to tell "empty"
// from "not loaded yet". A mutated `useState(true)` -> `useState(false)` (or
// the reverse) therefore changes behavior and must fail here.
//
// Every API call is replaced by a promise that never settles, so the only
// values a hook can have shown are its first render and what its mount effect
// synchronously does.
import { cleanup, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { useActivities } from './useActivities';
import { useAudit } from './useAudit';
import { useCircles } from './useCircles';
import { useContacts } from './useContacts';
import { useErrorAggregation } from './useErrorAggregation';
import { useGraph } from './useGraph';
import { useHouseholds } from './useHouseholds';
import { useJobRunHealth, useJobRunHistory } from './useJobRuns';
import { useNotes } from './useNotes';
import { useNotificationChannelHealth } from './useNotificationChannelHealth';
import { useOccasionEvents } from './useOccasionEvents';
import { useSubsystemHealth } from './useSubsystemHealth';
import { useSystemEvents } from './useSystemEvents';
import { useSystemStatus } from './useSystemStatus';
import { useTags } from './useTags';
import { useTimeline } from './useTimeline';
import { useWebAuthn } from './useWebAuthn';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

// Some hooks log the unauthenticated fetch failure; keep the output quiet.
beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
});

// Replaces every exported function of an API module with one that never settles.
const hangAll = vi.hoisted(
  () =>
    (actual: Record<string, unknown>): Record<string, unknown> =>
      Object.fromEntries(
        Object.entries(actual).map(([name, value]) => [
          name,
          typeof value === 'function' && /^[a-z]/.test(name)
            ? vi.fn(() => new Promise(() => {}))
            : value,
        ]),
      ),
);

vi.mock('../api/activities', async (orig) => hangAll(await orig()));
vi.mock('../api/audit', async (orig) => hangAll(await orig()));
vi.mock('../api/circles', async (orig) => hangAll(await orig()));
vi.mock('../api/contacts', async (orig) => hangAll(await orig()));
vi.mock('../api/errorAggregation', async (orig) => hangAll(await orig()));
vi.mock('../api/graph', async (orig) => hangAll(await orig()));
vi.mock('../api/households', async (orig) => hangAll(await orig()));
vi.mock('../api/jobRuns', async (orig) => hangAll(await orig()));
vi.mock('../api/notes', async (orig) => hangAll(await orig()));
vi.mock('../api/notificationHealth', async (orig) => hangAll(await orig()));
vi.mock('../api/occasionEvents', async (orig) => hangAll(await orig()));
vi.mock('../api/subsystemHealth', async (orig) => hangAll(await orig()));
vi.mock('../api/systemEvents', async (orig) => hangAll(await orig()));
vi.mock('../api/systemStatus', async (orig) => hangAll(await orig()));
vi.mock('../api/tags', async (orig) => hangAll(await orig()));
vi.mock('../api/timeline', async (orig) => hangAll(await orig()));
vi.mock('../api/webauthn', async (orig) => hangAll(await orig()));

// Renders `use` and returns the `loading` value of every render, in order.
function loadingHistory(use: () => { loading: boolean }): boolean[] {
  const seen: boolean[] = [];
  renderHook(() => {
    const r = use();
    seen.push(r.loading);
    return r;
  });
  return seen;
}

describe('hooks that start in the loading state', () => {
  const cases: Array<[string, () => { loading: boolean }]> = [
    ['useActivities', () => useActivities({}, 7)],
    ['useAudit', () => useAudit()],
    ['useCircles', () => useCircles()],
    ['useContacts', () => useContacts()],
    ['useErrorAggregation', () => useErrorAggregation()],
    ['useGraph', () => useGraph()],
    ['useJobRunHealth', () => useJobRunHealth()],
    ['useNotes', () => useNotes(7)],
    ['useNotificationChannelHealth', () => useNotificationChannelHealth()],
    ['useSubsystemHealth', () => useSubsystemHealth()],
    ['useSystemEvents', () => useSystemEvents()],
    ['useSystemStatus', () => useSystemStatus()],
    ['useWebAuthn', () => useWebAuthn()],
  ];

  test.each(cases)('%s reports loading=true on its very first render', (_name, use) => {
    expect(loadingHistory(use)[0]).toBe(true);
  });
});

describe('hooks that start idle', () => {
  const cases: Array<[string, () => { loading: boolean }]> = [
    ['useHouseholds', () => useHouseholds()],
    ['useJobRunHistory', () => useJobRunHistory()],
    ['useOccasionEvents', () => useOccasionEvents()],
    ['useTags', () => useTags()],
  ];

  test.each(cases)('%s reports loading=false on its very first render', (_name, use) => {
    expect(loadingHistory(use)[0]).toBe(false);
  });
});

describe('useTimeline', () => {
  test('first render is the idle, empty state', () => {
    const seen: Array<ReturnType<typeof useTimeline>> = [];
    renderHook(() => {
      const r = useTimeline(undefined);
      seen.push(r);
      return r;
    });
    const first = seen[0];
    expect(first.items).toEqual([]);
    expect(first.nextCursor).toBe('');
    expect(first.loading).toBe(false);
    expect(first.loadingMore).toBe(false);
  });
});
