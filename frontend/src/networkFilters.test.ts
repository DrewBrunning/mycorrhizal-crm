import { afterEach, describe, expect, test } from 'vitest';
import {
  DEFAULT_NETWORK_FILTERS,
  dropStaleCircleFilter,
  loadNetworkFilters,
  type NetworkFilters,
  saveNetworkFilters,
} from './networkFilters';

afterEach(() => {
  localStorage.clear();
});

describe('loadNetworkFilters', () => {
  test('returns the defaults when nothing is persisted', () => {
    expect(loadNetworkFilters()).toEqual(DEFAULT_NETWORK_FILTERS);
  });

  test('reads back every persisted field', () => {
    localStorage.setItem('network-selected-circle', 'Family');
    localStorage.setItem('network-show-relationships', 'false');
    localStorage.setItem('network-show-activities', 'false');
    localStorage.setItem('network-show-circles', 'true');
    localStorage.setItem('network-centered-node-id', 'c-42');

    expect(loadNetworkFilters()).toEqual({
      selectedCircle: 'Family',
      showRelationships: false,
      showActivities: false,
      showCircles: true,
      centeredNodeId: 'c-42',
    });
  });

  test('treats any value other than the literal string "false" as showRelationships/showActivities on', () => {
    localStorage.setItem('network-show-relationships', 'garbage');
    localStorage.setItem('network-show-activities', 'garbage');
    expect(loadNetworkFilters().showRelationships).toBe(true);
    expect(loadNetworkFilters().showActivities).toBe(true);
  });

  test('only the literal string "true" turns showCircles on', () => {
    localStorage.setItem('network-show-circles', 'garbage');
    expect(loadNetworkFilters().showCircles).toBe(false);
  });

  test('missing centeredNodeId loads as null', () => {
    expect(loadNetworkFilters().centeredNodeId).toBeNull();
  });
});

describe('saveNetworkFilters', () => {
  test('persists every field so loadNetworkFilters round-trips it', () => {
    const filters: NetworkFilters = {
      selectedCircle: 'Work',
      showRelationships: false,
      showActivities: true,
      showCircles: true,
      centeredNodeId: 'c-7',
    };

    saveNetworkFilters(filters);

    expect(loadNetworkFilters()).toEqual(filters);
  });

  test('removes the centeredNodeId key instead of writing "null"', () => {
    localStorage.setItem('network-centered-node-id', 'c-1');

    saveNetworkFilters({ ...DEFAULT_NETWORK_FILTERS, centeredNodeId: null });

    expect(localStorage.getItem('network-centered-node-id')).toBeNull();
  });

  test('persists an empty selectedCircle rather than leaving a stale value behind', () => {
    localStorage.setItem('network-selected-circle', 'Old Circle');

    saveNetworkFilters({ ...DEFAULT_NETWORK_FILTERS, selectedCircle: '' });

    expect(localStorage.getItem('network-selected-circle')).toBe('');
  });
});

describe('dropStaleCircleFilter', () => {
  test('leaves filters untouched (same reference) when no circle is selected', () => {
    const filters: NetworkFilters = { ...DEFAULT_NETWORK_FILTERS, selectedCircle: '' };
    expect(dropStaleCircleFilter(filters, ['Family', 'Work'])).toBe(filters);
  });

  test('leaves filters untouched (same reference) when the selected circle still exists', () => {
    const filters: NetworkFilters = { ...DEFAULT_NETWORK_FILTERS, selectedCircle: 'Family' };
    expect(dropStaleCircleFilter(filters, ['Family', 'Work'])).toBe(filters);
  });

  test('clears a selected circle that no longer exists', () => {
    const filters: NetworkFilters = {
      ...DEFAULT_NETWORK_FILTERS,
      selectedCircle: 'Deleted Circle',
    };
    expect(dropStaleCircleFilter(filters, ['Family', 'Work'])).toEqual({
      ...filters,
      selectedCircle: '',
    });
  });

  test('clears a selected circle when there are no circles at all', () => {
    const filters: NetworkFilters = { ...DEFAULT_NETWORK_FILTERS, selectedCircle: 'Gone' };
    expect(dropStaleCircleFilter(filters, [])).toEqual({ ...filters, selectedCircle: '' });
  });

  test('preserves the other fields when clearing a stale circle', () => {
    const filters: NetworkFilters = {
      selectedCircle: 'Deleted Circle',
      showRelationships: false,
      showActivities: false,
      showCircles: true,
      centeredNodeId: 'c-9',
    };
    expect(dropStaleCircleFilter(filters, [])).toEqual({ ...filters, selectedCircle: '' });
  });
});
