import { afterEach, describe, expect, test, vi } from 'vitest';
import {
  createPreference,
  GIFTS_TAB_SECTIONS,
  getPreferences,
  isGiftsTabCategory,
  OVERVIEW_TAB_SECTIONS,
  PREFERENCE_CATEGORY_CONFIG,
} from './preferences';

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('tab section taxonomy', () => {
  test('every section used by a category is claimed by exactly one of GIFTS_TAB_SECTIONS/OVERVIEW_TAB_SECTIONS', () => {
    const usedSections = new Set(PREFERENCE_CATEGORY_CONFIG.map((c) => c.section));
    for (const section of usedSections) {
      const inGifts = GIFTS_TAB_SECTIONS.includes(section);
      const inOverview = OVERVIEW_TAB_SECTIONS.includes(section);
      expect(
        inGifts !== inOverview,
        `section "${section}" must appear in exactly one of GIFTS_TAB_SECTIONS/OVERVIEW_TAB_SECTIONS (got gifts=${inGifts}, overview=${inOverview})`,
      ).toBe(true);
    }
  });

  test('isGiftsTabCategory matches the section split for every configured category', () => {
    for (const { category, section } of PREFERENCE_CATEGORY_CONFIG) {
      expect(isGiftsTabCategory(category)).toBe(GIFTS_TAB_SECTIONS.includes(section));
    }
  });

  test('an unrecognized category is not treated as a Gifts-tab category', () => {
    expect(isGiftsTabCategory('some_future_category')).toBe(false);
  });
});

describe('getPreferences', () => {
  test('requests the entity-scoped endpoint and parses the response', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        preferences: [
          {
            id: 'p1',
            entity_id: 'alice-uid',
            category: 'food',
            value: 'Vegetarian',
            sensitivity: 'normal',
          },
        ],
        total: 1,
        page: 1,
        limit: 100,
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const response = await getPreferences({ entityId: 'alice-uid' });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain('/preferences?');
    expect(url).toContain('entity_id=alice-uid');
    expect(response.total).toBe(1);
    expect(response.preferences[0].value).toBe('Vegetarian');
  });
});

describe('createPreference', () => {
  test('POSTs the input and unwraps the created preference', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        message: 'Preference created successfully',
        preference: {
          id: 'p1',
          entity_id: 'alice-uid',
          category: 'food',
          value: 'Vegan',
          sensitivity: 'normal',
        },
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await createPreference({
      entity_id: 'alice-uid',
      category: 'food',
      value: 'Vegan',
      sensitivity: 'normal',
    });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/preferences');
    expect(init.method).toBe('POST');
    expect(result.id).toBe('p1');
    expect(result.value).toBe('Vegan');
  });
});
