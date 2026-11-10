// Destinations for global-search hits (#2174), shared by the top-bar dropdown
// and the /search results page so the two can never disagree about where a
// hit opens.

export type SearchTab = 'contacts' | 'notes' | 'activities';

export function searchResultsHref(query: string, tab?: SearchTab): string {
  const params = new URLSearchParams({ q: query });
  if (tab && tab !== 'contacts') params.set('tab', tab);
  // URLSearchParams encodes a space as "+"; keep %20 so the URL reads the same
  // as every other encodeURIComponent-built link in the app.
  return `/search?${params.toString().replace(/\+/g, '%20')}`;
}

export function contactHref(id: number): string {
  return `/contacts/${id}`;
}

// A note belongs to a contact (open that contact's page, where notes are
// shown in context) or is unfiled (open the Notes page filtered to the query).
export function noteHref(note: { contact_id?: number | null }, query: string): string {
  if (note.contact_id != null) return contactHref(note.contact_id);
  return `/notes?search=${encodeURIComponent(query)}`;
}

// Activities have no detail page of their own: open the Activities list
// filtered by the same query that found the hit.
export function activityHref(query: string): string {
  return `/activities?search=${encodeURIComponent(query)}`;
}
