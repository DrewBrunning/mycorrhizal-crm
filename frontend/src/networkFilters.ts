// Persisted network-graph filter state (issue #350, port of upstream
// meerkat-crm#210). NetworkPage previously scattered these across individual
// useState/localStorage pairs, and selectedCircle wasn't persisted at all --
// it reset to "all circles" on every reload while the other three filters
// survived. Consolidating into one NetworkFilters object plus a single
// load/save pair keeps them all in sync and gives the stale-circle cleanup
// (below) one place to live.

export interface NetworkFilters {
  selectedCircle: string;
  showRelationships: boolean;
  showActivities: boolean;
  showCircles: boolean;
  centeredNodeId: string | null;
}

const STORAGE_KEYS = {
  selectedCircle: 'network-selected-circle',
  showRelationships: 'network-show-relationships',
  showActivities: 'network-show-activities',
  showCircles: 'network-show-circles',
  centeredNodeId: 'network-centered-node-id',
} as const;

export const DEFAULT_NETWORK_FILTERS: NetworkFilters = {
  selectedCircle: '',
  showRelationships: true,
  showActivities: true,
  showCircles: false,
  centeredNodeId: null,
};

export function loadNetworkFilters(): NetworkFilters {
  return {
    selectedCircle:
      localStorage.getItem(STORAGE_KEYS.selectedCircle) ?? DEFAULT_NETWORK_FILTERS.selectedCircle,
    showRelationships: localStorage.getItem(STORAGE_KEYS.showRelationships) !== 'false',
    showActivities: localStorage.getItem(STORAGE_KEYS.showActivities) !== 'false',
    showCircles: localStorage.getItem(STORAGE_KEYS.showCircles) === 'true',
    centeredNodeId: localStorage.getItem(STORAGE_KEYS.centeredNodeId),
  };
}

export function saveNetworkFilters(filters: NetworkFilters): void {
  localStorage.setItem(STORAGE_KEYS.selectedCircle, filters.selectedCircle);
  localStorage.setItem(STORAGE_KEYS.showRelationships, String(filters.showRelationships));
  localStorage.setItem(STORAGE_KEYS.showActivities, String(filters.showActivities));
  localStorage.setItem(STORAGE_KEYS.showCircles, String(filters.showCircles));
  if (filters.centeredNodeId !== null) {
    localStorage.setItem(STORAGE_KEYS.centeredNodeId, filters.centeredNodeId);
  } else {
    localStorage.removeItem(STORAGE_KEYS.centeredNodeId);
  }
}

/**
 * Drops a persisted selectedCircle that no longer matches any current circle
 * name (the circle was renamed or deleted since the filter was saved), so
 * the graph doesn't come up filtered down to nothing. Returns the same
 * object reference when no change is needed.
 */
export function dropStaleCircleFilter(
  filters: NetworkFilters,
  validCircleNames: readonly string[],
): NetworkFilters {
  if (filters.selectedCircle && !validCircleNames.includes(filters.selectedCircle)) {
    return { ...filters, selectedCircle: '' };
  }
  return filters;
}
