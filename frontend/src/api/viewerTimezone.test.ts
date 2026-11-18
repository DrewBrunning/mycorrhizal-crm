import { afterEach, describe, expect, test, vi } from 'vitest';
import { getUpcomingBirthdays } from './contacts';
import { getDashboard } from './dashboard';
import { getGiftShoppingList, getUpcomingOccasions } from './occasionObligations';
import { getUpcomingReminders } from './reminders';
import { viewerTimezone, withViewerTimezone } from './viewerTimezone';

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function stubZone(zone: unknown) {
  vi.spyOn(Intl, 'DateTimeFormat').mockImplementation(
    () => ({ resolvedOptions: () => ({ timeZone: zone }) }) as unknown as Intl.DateTimeFormat,
  );
}

function stubFetch() {
  const fetchMock = vi.fn().mockResolvedValue({
    ok: true,
    json: async () => ({ birthdays: [], occasions: [], reminders: [], gift_shopping_list: [] }),
  });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

const calls: [string, () => Promise<unknown>][] = [
  ['/dashboard', () => getDashboard()],
  ['/occasions/upcoming', () => getUpcomingOccasions()],
  ['/occasion-obligations/gift-shopping-list', () => getGiftShoppingList()],
  ['/contacts/birthdays', () => getUpcomingBirthdays()],
  ['/reminders/upcoming', () => getUpcomingReminders()],
];

describe('viewerTimezone', () => {
  test('returns the browser IANA zone', () => {
    stubZone('Pacific/Auckland');
    expect(viewerTimezone()).toBe('Pacific/Auckland');
  });

  test.each([undefined, '', '   ', 'Local', 'x'.repeat(65)])(
    'omits an unusable zone (%s)',
    (zone) => {
      stubZone(zone);
      expect(viewerTimezone()).toBeUndefined();
    },
  );

  test('omits the zone when Intl throws', () => {
    vi.spyOn(Intl, 'DateTimeFormat').mockImplementation(() => {
      throw new Error('no Intl');
    });
    expect(viewerTimezone()).toBeUndefined();
  });

  test('withViewerTimezone appends with ? or & as needed', () => {
    stubZone('America/Chicago');
    expect(withViewerTimezone('/a')).toBe('/a?tz=America%2FChicago');
    expect(withViewerTimezone('/a?days=30')).toBe('/a?days=30&tz=America%2FChicago');
  });
});

describe.each(calls)('%s', (path, call) => {
  test('sends the browser zone as tz', async () => {
    stubZone('Pacific/Auckland');
    const fetchMock = stubFetch();
    await call();
    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain(path);
    expect(new URL(url, 'http://x').searchParams.get('tz')).toBe('Pacific/Auckland');
  });

  test('omits tz when the browser zone is unusable', async () => {
    stubZone(undefined);
    const fetchMock = stubFetch();
    await call();
    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain(path);
    expect(url).not.toContain('tz=');
  });
});
