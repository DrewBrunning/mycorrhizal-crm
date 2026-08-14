// Contact-related API calls
import { apiFetch, API_BASE_URL, getAuthHeaders, parseErrorResponse } from './client';

export interface ContactValue {
  type: string;
  value: string;
  // Rich-field passthrough (WP11, T29): the flat editing shape only exposes
  // type+value, so pref/label/features/extra-contexts are carried alongside
  // and re-emitted on save rather than silently dropped (the same pattern
  // ContactAddress.passthrough uses for non-standard address components).
  pref?: number | null;
  label?: string;
  contexts?: string[];
  features?: string[];
}

export interface ContactAddress {
  type: string;
  street: string;
  city: string;
  region: string;
  postal: string;
  country: string;
  // Sub-street parts (T79, backend ticket 123): vCard ADR slots 1-2 (PO box /
  // extended address) plus RFC 9554's floor, the ones a person actually
  // types. The remaining kinds (room, building, block, district, number,
  // ...) have no flat slot and ride `passthrough` instead. Keep in sync with
  // backend/models/contact.go's ContactAddress.
  pobox?: string;
  apartment?: string;
  floor?: string;
  // passthrough preserves address components whose kind is not one of the
  // fields rendered above (room, building, district, landmark, etc.) so they
  // survive an edit-and-save cycle through the flat editing shape (T25).
  passthrough?: CardAddressComponent[];
  // Rich-field passthrough (WP11, T29): coordinates/timeZone/pref/full etc.
  // are carried alongside and re-emitted on save rather than dropped.
  coordinates?: string;
  timeZone?: string;
  pref?: number | null;
  full?: string;
}

// Contact is the flat shape every existing component (ContactDetailPage,
// AddContactDialog, ContactInformation, ContactHeader, ...) still consumes.
// The backend no longer speaks this shape on the wire -- see the
// toLegacyContact / toContactRecordInput adapter below, which is the
// temporary translation layer that lets those components keep working
// unmodified while the wire format underneath is the real nested card/crm
// shape. This shim (and this Contact type) goes away once every consumer
// is migrated to the nested model directly (see the frontend migration
// task list); do not add new features here that assume it's permanent.
export interface Contact {
  ID: number;
  // Contact.VCardUID -- surfaced for callers that need the stable UUID
  // rather than the numeric ID (e.g. RelationshipEdge.SourceID/TargetID,
  // api/relationshipEdges.ts). Already fetched server-side on every summary
  // response; just not previously exposed on this shape.
  uid?: string;
  firstname: string;
  lastname: string;
  nickname?: string;
  gender?: string;
  email?: string;
  phone?: string;
  birthday?: string;
  photo?: string;
  address?: string;
  how_we_met?: string;
  work_information?: string;
  contact_information?: string;
  // CRMEnvelope.Kind (T27): individual|pet|animal. Survives on the flat
  // shape solely for toContactRecordInput's flat->nested mapping.
  kind?: string;
  photo_thumbnail?: string;
  archived?: boolean;
  // Multi-valued vCard fields
  emails?: ContactValue[];
  phones?: ContactValue[];
  addresses?: ContactAddress[];
  urls?: ContactValue[];
  impps?: ContactValue[];
  // Structured name parts
  prefix?: string;
  middle_name?: string;
  suffix?: string;
  // Organizational fields
  organization?: string;
  department?: string;
  job_title?: string;
  role?: string;
  anniversary?: string;
}

// ---------------------------------------------------------------------------
// Nested wire types (mirror backend/openapi.yaml's Card/CRMEnvelope/
// ContactRecordInput/ContactRecordResponse schemas). Deliberately partial --
// only the fields the adapter below actually reads/writes are typed; the
// backend may send more (e.g. localizations, relatedTo) that this shim
// ignores rather than losing the app's ability to build if a field is
// missing here.
// ---------------------------------------------------------------------------

export interface NameComponent {
  kind: 'title' | 'given' | 'given2' | 'surname' | 'surname2' | 'credential' | 'generation' | 'separator';
  value: string;
  phonetic?: string;
}

export interface CardName {
  components?: NameComponent[];
  full?: string;
  sortAs?: Record<string, string>;
  isOrdered?: boolean;
  defaultSeparator?: string;
  phoneticSystem?: string;
  phoneticScript?: string;
}

export interface CardNickname {
  id?: string;
  name: string;
  contexts?: string[];
  pref?: number | null;
}

export interface CardOrgUnit {
  name: string;
  sortAs?: string;
}

export interface CardOrganization {
  id?: string;
  name?: string;
  units?: CardOrgUnit[];
  sortAs?: string;
}

export interface CardTitle {
  id?: string;
  name: string;
  kind?: 'title' | 'role';
  organizationId?: string;
}

export interface CardEmail {
  id?: string;
  address: string;
  contexts?: string[];
  pref?: number | null;
  label?: string;
}

export interface CardPhone {
  id?: string;
  number: string;
  features?: string[];
  contexts?: string[];
  pref?: number | null;
  label?: string;
}

export interface CardOnlineService {
  id?: string;
  uri?: string;
  service?: string;
  user?: string;
  contexts?: string[];
  pref?: number | null;
  label?: string;
}

export interface CardResource {
  id?: string;
  uri: string;
  kind?: string;
  mediaType?: string;
  label?: string;
  contexts?: string[];
  pref?: number | null;
  listAs?: number | null;
}

export interface CardAddressComponent {
  kind: string;
  value: string;
  phonetic?: string;
}

export interface CardAddress {
  id?: string;
  components?: CardAddressComponent[];
  countryCode?: string;
  coordinates?: string;
  timeZone?: string;
  contexts?: string[];
  pref?: number | null;
  full?: string;
  isOrdered?: boolean;
  defaultSeparator?: string;
  phoneticSystem?: string;
  phoneticScript?: string;
}

export interface CardPartialDate {
  year?: number | null;
  month?: number | null;
  day?: number | null;
  calendarScale?: string;
}

export interface CardAnniversaryDate {
  partial?: CardPartialDate;
  timestamp?: string | null;
}

export interface CardAnniversary {
  id?: string;
  kind: 'birth' | 'death' | 'wedding';
  date: CardAnniversaryDate;
  place?: CardAddress;
}

export interface CardGrammaticalGender {
  id?: string;
  value: string;
  language?: string;
}

export interface CardPronouns {
  id?: string;
  pronouns: string;
  contexts?: string[];
  pref?: number | null;
}

export interface CardSpeakToAs {
  grammaticalGenders?: CardGrammaticalGender[];
  pronouns?: CardPronouns[];
}

export interface CardPersonalInfo {
  id?: string;
  kind: string;
  value: string;
  level?: string;
  listAs?: number | null;
  label?: string;
}

export interface CardAuthor {
  name?: string;
  uri?: string;
}

export interface CardNote {
  id?: string;
  note: string;
  author?: CardAuthor;
  created?: { utc: string };
}

export interface CardLanguagePref {
  id?: string;
  language: string;
  contexts?: string[];
  pref?: number | null;
}

export interface CardRelation {
  target: string;
  relations?: string[];
}

export interface Card {
  uid?: string;
  kind?: string;
  language?: string;
  prodId?: string;
  created?: { utc: string };
  updated?: { utc: string };
  name?: CardName;
  nicknames?: CardNickname[];
  organizations?: CardOrganization[];
  titles?: CardTitle[];
  emails?: CardEmail[];
  phones?: CardPhone[];
  imppAddresses?: CardOnlineService[];
  socialProfiles?: CardOnlineService[];
  otherOnlineServices?: CardOnlineService[];
  addresses?: CardAddress[];
  anniversaries?: CardAnniversary[];
  speakToAs?: CardSpeakToAs;
  personalInfo?: CardPersonalInfo[];
  notes?: CardNote[];
  keywords?: string[];
  media?: CardResource[];
  calendars?: CardResource[];
  freeBusyUrls?: CardResource[];
  schedulingAddresses?: CardResource[];
  cryptoKeys?: CardResource[];
  directories?: CardResource[];
  links?: CardResource[];
  contactUris?: CardResource[];
  preferredLanguages?: CardLanguagePref[];
  relatedTo?: CardRelation[];
  members?: string[];
  localizations?: Record<string, unknown>;
}

// CRMEnvelope mirrors the backend's contactmodel.CRMEnvelope. kind is the
// envelope-side entity kind (individual|pet|animal, T27): it drives the
// household suggestion engine's pet/animal classification
// (services/household_service.go's classifyMember), so it must stay in sync
// with the backend's accepted tokens — there is no dynamic type-list endpoint
// (see CLAUDE.md frontend trap #4).
export interface CRMEnvelope {
  kind?: string;
  how_we_met?: string;
  work_information?: string;
  contact_information?: string;
}

export interface ContactRecordInput {
  gender?: string;
  card: Card;
  crm: CRMEnvelope;
}

export interface ContactRecordResponse {
  id: number;
  uid: string;
  etag: string;
  gender?: string;
  card: Card;
  crm: CRMEnvelope;
  photo?: string;
  photo_thumbnail?: string;
  archived?: boolean;
}

// ContactSummaryDTO mirrors the backend's slim GET /contacts list
// projection exactly (models.ContactSummary) -- distinct from the legacy
// Contact shape above, which the adapter maps summaries down into. Hand-
// synced with the Go struct field for field (no dynamic schema endpoint
// exists anywhere in this codebase, per /CLAUDE.md frontend trap #4) --
// deliberately has no `circles` field: T108 removed it backend-side (it was
// never selected by the list query, and would have been the stale legacy
// flat column even if it had been), and the list's circle chips have always
// come from a separate useCircles() lookup, not this DTO.
interface ContactSummaryDTO {
  id: number;
  uid: string;
  firstname: string;
  lastname: string;
  nickname: string;
  fn: string;
  primary_email: string;
  primary_phone: string;
  birthday: string;
  org: string;
  photo: string;
  photo_thumbnail: string;
  archived: boolean;
}

// ---------------------------------------------------------------------------
// Adapter: nested wire shape <-> legacy flat Contact shape.
// ---------------------------------------------------------------------------

export function nameComponentValue(components: NameComponent[] | undefined, kind: NameComponent['kind']): string | undefined {
  return components?.find((c) => c.kind === kind)?.value;
}

// formatAnniversaryDate turns a CardAnniversaryDate into the ISO YYYY-MM-DD
// / year-less --MM-DD string convention used throughout this app for
// birthday/anniversary fields (never a full RFC3339 timestamp for a
// date-only field).
export function formatAnniversaryDate(date: CardAnniversaryDate | undefined): string | undefined {
  if (!date) return undefined;
  if (date.partial) {
    const { year, month, day } = date.partial;
    const mm = month != null ? String(month).padStart(2, '0') : undefined;
    const dd = day != null ? String(day).padStart(2, '0') : undefined;
    if (mm && dd) {
      return year != null ? `${String(year).padStart(4, '0')}-${mm}-${dd}` : `--${mm}-${dd}`;
    }
  }
  if (date.timestamp) {
    return date.timestamp.slice(0, 10);
  }
  return undefined;
}

// parseAnniversaryDate is formatAnniversaryDate's inverse: a
// YYYY-MM-DD or --MM-DD string into a CardAnniversaryDate.
export function parseAnniversaryDate(value: string): CardAnniversaryDate {
  const yearLess = /^--(\d{2})-(\d{2})$/.exec(value);
  if (yearLess) {
    return { partial: { month: parseInt(yearLess[1], 10), day: parseInt(yearLess[2], 10) } };
  }
  const full = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value);
  if (full) {
    return { partial: { year: parseInt(full[1], 10), month: parseInt(full[2], 10), day: parseInt(full[3], 10) } };
  }
  // Unparseable input: pass through as a raw timestamp rather than
  // dropping it silently, matching the backend's own degradation policy.
  return { timestamp: value };
}

// ---------------------------------------------------------------------------
// Per-concept Card <-> ContactValue/ContactAddress helpers. Shared by
// toLegacyContact/toContactRecordInput below AND by components that have
// migrated to reading/writing Card/CRMEnvelope directly (ContactInformation)
// -- a single source of truth for the mapping rules so a migrated
// component's edits can never drift from what the shim itself would produce.
// ---------------------------------------------------------------------------

export function cardEmailsToValues(emails: CardEmail[] | undefined): ContactValue[] {
  return (emails || []).map((e) => ({
    type: e.contexts?.[0] || '',
    value: e.address,
    pref: e.pref ?? undefined,
    label: e.label || undefined,
    contexts: e.contexts,
  }));
}
export function valuesToCardEmails(values: ContactValue[]): CardEmail[] {
  return values.filter((e) => e.value.trim()).map((e) => ({
    address: e.value,
    contexts: e.contexts?.length ? e.contexts : e.type ? [e.type] : undefined,
    pref: e.pref,
    label: e.label || undefined,
  }));
}

export function cardPhonesToValues(phones: CardPhone[] | undefined): ContactValue[] {
  return (phones || []).map((p) => ({
    type: p.features?.[0] || p.contexts?.[0] || '',
    value: p.number,
    pref: p.pref ?? undefined,
    label: p.label || undefined,
    contexts: p.contexts,
    features: p.features,
  }));
}
export function valuesToCardPhones(values: ContactValue[]): CardPhone[] {
  return values.filter((p) => p.value.trim()).map((p) => ({
    number: p.value,
    features: p.features?.length ? p.features : undefined,
    contexts: p.contexts?.length ? p.contexts : p.type ? [p.type] : undefined,
    pref: p.pref ?? undefined,
    label: p.label || undefined,
  }));
}

export function cardLinksToValues(links: CardResource[] | undefined): ContactValue[] {
  return (links || []).map((l) => ({
    type: l.contexts?.[0] || '',
    value: l.uri,
    pref: l.pref ?? undefined,
    label: l.label || undefined,
    contexts: l.contexts,
  }));
}
export function valuesToCardLinks(values: ContactValue[]): CardResource[] {
  return values.filter((u) => u.value.trim()).map((u) => ({
    uri: u.value,
    contexts: u.contexts?.length ? u.contexts : u.type ? [u.type] : undefined,
    pref: u.pref,
    label: u.label || undefined,
  }));
}

export function cardImppToValues(impps: CardOnlineService[] | undefined): ContactValue[] {
  return (impps || []).map((i) => ({
    type: i.contexts?.[0] || '',
    value: i.uri || '',
    pref: i.pref ?? undefined,
    label: i.label || undefined,
    contexts: i.contexts,
  }));
}
export function valuesToCardImpp(values: ContactValue[]): CardOnlineService[] {
  return values.filter((i) => i.value.trim()).map((i) => ({
    uri: i.value,
    contexts: i.contexts?.length ? i.contexts : i.type ? [i.type] : undefined,
    pref: i.pref,
    label: i.label || undefined,
  }));
}

// ---------------------------------------------------------------------------
// OnlineService helpers (WP3). Shared by SocialProfiles, OtherOnlineServices
// and the upgraded IMPP editor. The rich struct carries service/user/uri and
// full context/pref/label; only uri+service are surfaced as direct inputs —
// everything else is preserved through the round trip by the nested editors.
// ---------------------------------------------------------------------------

export interface OnlineServiceRow {
  id?: string;
  service: string;
  uri: string;
  user: string;
  label: string;
  contexts: string[];
  pref?: number | null;
}

export function onlineServicesToRows(services: CardOnlineService[] | undefined): OnlineServiceRow[] {
  return (services || []).map((s) => ({
    id: s.id,
    service: s.service || '',
    uri: s.uri || '',
    user: s.user || '',
    label: s.label || '',
    contexts: s.contexts || [],
    pref: s.pref,
  }));
}

export function rowsToOnlineServices(rows: OnlineServiceRow[]): CardOnlineService[] {
  return rows
    .filter((r) => r.uri.trim() || r.service.trim() || r.user.trim())
    .map((r) => ({
      id: r.id,
      uri: r.uri.trim() || undefined,
      service: r.service.trim() || undefined,
      user: r.user.trim() || undefined,
      label: r.label.trim() || undefined,
      contexts: r.contexts.length > 0 ? r.contexts : undefined,
      pref: r.pref,
    }));
}

// ctx2type (docs/fork-plan/20-correspondence.md §20.4), mirroring the Go
// contactmodel.ContextToTypeToken map. The neutral model's Contexts vocabulary
// is private/work; the flat shape this file's adapters produce — and the
// `contacts.types.*` i18n keys that render it — use home/work.
//
// T91: without this, an address imported from a vCard `ADR;TYPE=home` (which
// the importer correctly stores as `contexts: ["private"]`) rendered as the
// literal string "private", because no `contacts.types.private` key exists and
// i18next falls back to the raw token. The backend fix corrected the persisted
// flat column; the contact detail page reads the nested Card instead, so it
// needed the same translation here.
//
// The fall-through on a miss is load-bearing: Contexts is not a closed enum on
// the write side, so an unrecognized value is user data to show, not to blank.
const ADDRESS_CONTEXT_TO_TYPE: Record<string, string> = {
  private: 'home',
  work: 'work',
  billing: 'billing',
  delivery: 'delivery',
};

export function cardAddressesToValues(addresses: CardAddress[] | undefined): ContactAddress[] {
  return (addresses || []).map((a) => {
    const comps = a.components || [];
    const find = (kind: string) => comps.find((c) => c.kind === kind)?.value || '';
    // Preserve components not mapped to the rendered fields so they survive
    // an edit-and-save round trip (T25). T79 added the three sub-street kinds
    // to the flat shape, so they no longer ride passthrough.
    const knownKinds = new Set(['name', 'number', 'locality', 'region', 'postcode', 'country', 'postOfficeBox', 'apartment', 'floor']);
    const passthrough = comps.filter((c) => !knownKinds.has(c.kind));
    return {
      type: a.contexts?.[0] ? (ADDRESS_CONTEXT_TO_TYPE[a.contexts[0]] ?? a.contexts[0]) : '',
      street: find('name') || find('number'),
      city: find('locality'),
      region: find('region'),
      postal: find('postcode'),
      country: find('country') || a.countryCode || '',
      pobox: find('postOfficeBox') || undefined,
      apartment: find('apartment') || undefined,
      floor: find('floor') || undefined,
      passthrough: passthrough.length > 0 ? passthrough : undefined,
      coordinates: a.coordinates,
      timeZone: a.timeZone,
      pref: a.pref,
      full: a.full,
    };
  });
}
export function valuesToCardAddresses(values: ContactAddress[]): CardAddress[] {
  return values
    .filter((a) => a.street.trim() || a.city.trim() || a.region.trim() || a.postal.trim() || a.country.trim() ||
      a.pobox?.trim() || a.apartment?.trim() || a.floor?.trim())
    .map((a) => {
      const components: CardAddressComponent[] = [];
      if (a.street) components.push({ kind: 'name', value: a.street });
      if (a.pobox) components.push({ kind: 'postOfficeBox', value: a.pobox });
      if (a.apartment) components.push({ kind: 'apartment', value: a.apartment });
      if (a.floor) components.push({ kind: 'floor', value: a.floor });
      if (a.city) components.push({ kind: 'locality', value: a.city });
      if (a.region) components.push({ kind: 'region', value: a.region });
      if (a.postal) components.push({ kind: 'postcode', value: a.postal });
      if (a.country) components.push({ kind: 'country', value: a.country });
      // Re-emit passthrough components that were preserved from the original address
      if (a.passthrough) components.push(...a.passthrough);
      return {
        components,
        contexts: a.type ? [a.type] : undefined,
        coordinates: a.coordinates,
        timeZone: a.timeZone,
        pref: a.pref,
        full: a.full,
      };
    });
}

export function getAnniversaryField(anniversaries: CardAnniversary[] | undefined, kind: 'birth' | 'wedding'): string | undefined {
  return formatAnniversaryDate((anniversaries || []).find((a) => a.kind === kind)?.date);
}
// withAnniversary replaces the single entry of `kind` (if any) with one
// parsed from `value`, or drops it entirely when `value` is empty -- the
// other kind (birth vs. wedding) is left untouched.
export function withAnniversary(
  anniversaries: CardAnniversary[] | undefined,
  kind: 'birth' | 'wedding',
  value: string
): CardAnniversary[] {
  const rest = (anniversaries || []).filter((a) => a.kind !== kind);
  return value ? [...rest, { kind, date: parseAnniversaryDate(value) }] : rest;
}

// Only the first organization/title entries are surfaced -- this app's UI
// (like the legacy Contact shape it grew from) only ever edits one
// organization and one title/role pair, even though the nested model
// supports arrays of both.
export function getOrganizationFields(organizations: CardOrganization[] | undefined): { organization?: string; department?: string } {
  const org = organizations?.[0];
  return { organization: org?.name, department: org?.units?.[0]?.name };
}
export function withOrganization(organization: string, department: string): CardOrganization[] {
  return organization ? [{ name: organization, units: department ? [{ name: department }] : undefined }] : [];
}

export function getTitleField(titles: CardTitle[] | undefined, kind: 'title' | 'role'): string | undefined {
  if (kind === 'title') return titles?.find((t) => t.kind === 'title' || !t.kind)?.name;
  return titles?.find((t) => t.kind === 'role')?.name;
}
export function withTitles(jobTitle: string, role: string): CardTitle[] {
  const titles: CardTitle[] = [];
  if (jobTitle) titles.push({ name: jobTitle, kind: 'title' });
  if (role) titles.push({ name: role, kind: 'role' });
  return titles;
}

// summaryToLegacyContact maps the slim GET /contacts list item shape down
// into the same flat Contact shape ContactsPage/DashboardPage etc. render.
function summaryToLegacyContact(summary: ContactSummaryDTO): Contact {
  return {
    ID: summary.id,
    uid: summary.uid,
    firstname: summary.firstname,
    lastname: summary.lastname,
    nickname: summary.nickname || undefined,
    email: summary.primary_email || undefined,
    phone: summary.primary_phone || undefined,
    birthday: summary.birthday || undefined,
    photo: summary.photo || undefined,
    photo_thumbnail: summary.photo_thumbnail || undefined,
    organization: summary.org || undefined,
    archived: summary.archived,
  };
}

// toContactRecordInput builds the nested request body from the flat
// form-data shape AddContactDialog/ContactDetailPage construct today.
export function toContactRecordInput(data: Partial<Contact>): ContactRecordInput {
  const nameComponents: NameComponent[] = [];
  if (data.prefix) nameComponents.push({ kind: 'title', value: data.prefix });
  if (data.firstname) nameComponents.push({ kind: 'given', value: data.firstname });
  if (data.middle_name) nameComponents.push({ kind: 'given2', value: data.middle_name });
  if (data.lastname) nameComponents.push({ kind: 'surname', value: data.lastname });
  if (data.suffix) nameComponents.push({ kind: 'generation', value: data.suffix });

  const emailValues = data.emails && data.emails.length > 0 ? data.emails : data.email ? [{ type: '', value: data.email }] : [];
  const phoneValues = data.phones && data.phones.length > 0 ? data.phones : data.phone ? [{ type: '', value: data.phone }] : [];
  const addressValues = data.addresses && data.addresses.length > 0
    ? data.addresses
    : data.address
      ? [{ type: '', street: data.address, city: '', region: '', postal: '', country: '' }]
      : [];

  const emails = valuesToCardEmails(emailValues);
  const phones = valuesToCardPhones(phoneValues);
  const links = valuesToCardLinks(data.urls || []);
  const imppAddresses = valuesToCardImpp(data.impps || []);
  const addresses = valuesToCardAddresses(addressValues);

  const anniversaries = withAnniversary(
    withAnniversary(undefined, 'birth', data.birthday || ''),
    'wedding',
    data.anniversary || ''
  );

  const organizations = withOrganization(data.organization || '', data.department || '');
  const titles = withTitles(data.job_title || '', data.role || '');

  return {
    gender: data.gender,
    card: {
      name: nameComponents.length > 0 ? { components: nameComponents } : undefined,
      nicknames: data.nickname ? [{ name: data.nickname }] : undefined,
      emails: emails.length > 0 ? emails : undefined,
      phones: phones.length > 0 ? phones : undefined,
      links: links.length > 0 ? links : undefined,
      imppAddresses: imppAddresses.length > 0 ? imppAddresses : undefined,
      addresses: addresses.length > 0 ? addresses : undefined,
      anniversaries: anniversaries.length > 0 ? anniversaries : undefined,
      organizations: organizations.length > 0 ? organizations : undefined,
      titles: titles.length > 0 ? titles : undefined,
    },
    crm: {
      kind: data.kind || undefined,
      how_we_met: data.how_we_met,
      work_information: data.work_information,
      contact_information: data.contact_information,
    },
  };
}

export interface Birthday {
  type: 'contact';
  name: string;
  birthday: string;
  photo_thumbnail?: string;
  contact_id: number;
}

export interface ContactsResponse {
  contacts: Contact[];
  // T17 cursor pagination: opaque resume token for the next page; empty when
  // there are no more rows. There is no total/page — cursor pagination gives
  // up the exact count on large tables.
  next_cursor: string;
  limit: number;
  // T103: present only while ?has_contact_info=true — how many contacts
  // matched the other filters but were excluded by the contact-info
  // predicate, so the UI can disclose that rows are hidden.
  hidden_count?: number;
}

export interface GetContactsParams {
  // Opaque resume token returned as next_cursor by the previous page.
  cursor?: string;
  limit?: number;
  search?: string;
  circle?: string;
  // Sort key (T73): "updated_at" (default, the (updated_at, id) cursor) or
  // "name" (the denormalized sort_name key, (sort_name, id) cursor).
  sort?: 'updated_at' | 'name';
  // Direction for the chosen sort's cursor order ("desc" default server-side).
  order?: 'asc' | 'desc';
  includeArchived?: boolean;
  archived?: boolean;
  // T103: when true, only contacts with at least one non-empty email, phone,
  // or URL are returned (the web Contacts page opts in by default). False and
  // absent both mean "show everything"; the server rejects any other value.
  hasContactInfo?: boolean;
}

// Get a page of contacts with filters, resumable via next_cursor (T17).
export async function getContacts(
  params: GetContactsParams
): Promise<ContactsResponse> {
  const { cursor, limit = 25, search = '', circle = '', sort, order, includeArchived, archived, hasContactInfo } = params;

  const queryParams = new URLSearchParams({
    limit: limit.toString(),
  });

  if (cursor) queryParams.append('cursor', cursor);
  if (search) queryParams.append('search', search);
  if (circle) queryParams.append('circle', circle);
  if (sort) queryParams.append('sort', sort);
  if (order) queryParams.append('order', order);
  if (includeArchived) queryParams.append('include_archived', 'true');
  if (archived !== undefined) queryParams.append('archived', archived.toString());
  if (hasContactInfo !== undefined) queryParams.append('has_contact_info', hasContactInfo.toString());

  const response = await apiFetch(
    `${API_BASE_URL}/contacts?${queryParams.toString()}`,
    { headers: getAuthHeaders() }
  );

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  const data: { contacts: ContactSummaryDTO[]; next_cursor: string; limit: number; hidden_count?: number } = await response.json();
  return {
    contacts: data.contacts.map(summaryToLegacyContact),
    next_cursor: data.next_cursor,
    limit: data.limit,
    hidden_count: data.hidden_count,
  };
}

// getAllContacts follows next_cursor until the list is exhausted — the
// "pull everything" affordance callers like the activity/network pages and
// timeline editor need now that there is no page=1000 shortcut and no total
// to size a loop with.
export async function getAllContacts(params: Omit<GetContactsParams, 'cursor'> = {}): Promise<Contact[]> {
  const contacts: Contact[] = [];
  let cursor: string | undefined;
  for (let guard = 0; guard < 100; guard++) {
    const page = await getContacts({ ...params, cursor });
    contacts.push(...page.contacts);
    cursor = page.next_cursor || undefined;
    if (!cursor) break;
  }
  return contacts;
}

// Resolves a batch of Contact.VCardUID values to full Contact objects in one
// request, via GET /contacts' ?vcard_uid= filter (§3d WP0). Needed because
// RelationshipEdge.SourceID/TargetID (api/relationshipEdges.ts) are bare
// VCardUID strings with no nested contact data. includeArchived: true so an
// edge pointing at a since-archived contact still resolves instead of
// silently vanishing (GetContacts excludes archived by default).
export async function getContactsByUid(uids: string[]): Promise<Map<string, Contact>> {
  const wanted = uids.filter(Boolean);
  const map = new Map<string, Contact>();
  if (wanted.length === 0) return map;

  const queryParams = new URLSearchParams({ include_archived: 'true' });
  wanted.forEach((uid) => queryParams.append('vcard_uid', uid));

  const response = await apiFetch(`${API_BASE_URL}/contacts?${queryParams.toString()}`, { headers: getAuthHeaders() });
  if (!response.ok) throw await parseErrorResponse(response);

  const data: { contacts: ContactSummaryDTO[] } = await response.json();
  for (const dto of data.contacts) {
    map.set(dto.uid, summaryToLegacyContact(dto));
  }
  return map;
}

// getContactRecord/updateContactRecord/createContactRecord read and write
// Card/CRMEnvelope directly. Every contact-editing component has migrated
// onto these (see docs/fork-plan/95, Tier 0 items 3-6) -- toContactRecordInput
// (below) still exists for the e2e test fixtures' convenience, but nothing
// in the app itself round-trips a full record through the flat Contact shape
// anymore.
export async function getContactRecord(id: string | number): Promise<ContactRecordResponse> {
  const response = await apiFetch(
    `${API_BASE_URL}/contacts/${id}`,
    { headers: getAuthHeaders() }
  );

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  return response.json();
}

export async function updateContactRecord(id: string | number, input: ContactRecordInput): Promise<ContactRecordResponse> {
  const response = await apiFetch(
    `${API_BASE_URL}/contacts/${id}`,
    {
      method: 'PUT',
      headers: getAuthHeaders(),
      body: JSON.stringify(input),
    }
  );

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  return response.json();
}

export async function createContactRecord(input: ContactRecordInput): Promise<ContactRecordResponse> {
  const response = await apiFetch(
    `${API_BASE_URL}/contacts`,
    {
      method: 'POST',
      headers: getAuthHeaders(),
      body: JSON.stringify(input),
    }
  );

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  const result = await response.json();
  return result.contact || result;
}

// Delete contact
export async function deleteContact(
  id: string | number
): Promise<void> {
  const response = await apiFetch(
    `${API_BASE_URL}/contacts/${id}`,
    {
      method: 'DELETE',
      headers: getAuthHeaders(),
    }
  );

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }
}

// Get contact profile picture
export async function getContactProfilePicture(
  id: string | number,
  thumbnail: boolean = false
): Promise<Blob | null> {
  const url = thumbnail
    ? `${API_BASE_URL}/contacts/${id}/profile_picture?thumbnail=true`
    : `${API_BASE_URL}/contacts/${id}/profile_picture`;
  const response = await apiFetch(url);

  if (!response.ok) {
    return null;
  }

  return response.blob();
}

// Upload contact profile picture
export async function uploadProfilePicture(
  id: string | number,
  imageBlob: Blob
): Promise<void> {
  const formData = new FormData();
  formData.append('photo', imageBlob, 'profile.jpg');

  const response = await apiFetch(
    `${API_BASE_URL}/contacts/${id}/profile_picture`,
    {
      method: 'POST',
      body: formData
    }
  );

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }
}

// Get all circles
export async function getCircles(): Promise<string[]> {
  const response = await apiFetch(
    `${API_BASE_URL}/contacts/circles`,
    { headers: getAuthHeaders() }
  );

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  const data = await response.json();
  // Backend returns array directly, not wrapped in object
  return Array.isArray(data) ? data : [];
}

// Temporary: reads legacy strings from the old flat Contact.Circles JSON
// column. Used by the T2 triage page during migration. Remove after migration.
export async function getLegacyCircles(): Promise<string[]> {
  const response = await apiFetch(
    `${API_BASE_URL}/contacts/circles?legacy=true`,
    { headers: getAuthHeaders() }
  );
  if (!response.ok) throw await parseErrorResponse(response);
  const data = await response.json();
  return Array.isArray(data) ? data : [];
}

// Temporary: filters contacts by a legacy flat-circle string. Used by the
// T2 triage page's member-add step. Remove after migration.
export async function getContactsByLegacyCircle(circle: string): Promise<{ contacts: Contact[]; total?: number }> {
  const queryParams = new URLSearchParams({
    limit: '500', circle_legacy: circle,
  });
  const response = await apiFetch(
    `${API_BASE_URL}/contacts?${queryParams.toString()}`,
    { headers: getAuthHeaders() }
  );
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

// Get random contacts (returns 5 contacts). NOTE: unlike every other
// endpoint in this file, GetContactsRandom was deliberately left out of the
// WP-71 nested-Card API migration on the backend (see
// docs/fork-plan/50-integration-and-rebrand.md) -- it still serializes
// models.Contact's raw GORM struct directly, which is already the flat
// legacy shape (down to gorm.Model's untagged "ID" field matching this
// type's capital ID). Do NOT route this through toLegacyContact/
// ContactRecordResponse -- there is no card/crm nesting to unwrap here.
export async function getRandomContacts(): Promise<Contact[]> {
  const response = await apiFetch(
    `${API_BASE_URL}/contacts/random`,
    { headers: getAuthHeaders() }
  );

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  const data = await response.json();
  return data.contacts || [];
}

// Get upcoming birthdays (returns up to 10 birthdays for contacts)
export async function getUpcomingBirthdays(): Promise<Birthday[]> {
  const response = await apiFetch(
    `${API_BASE_URL}/contacts/birthdays`,
    { headers: getAuthHeaders() }
  );

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  const data = await response.json();
  return data.birthdays || [];
}

// Archive a contact (deletes all reminders). Like GetContactsRandom above,
// ArchiveContact/UnarchiveContact were deliberately left out of the WP-71
// nested-Card API migration and still return models.Contact's raw flat
// JSON directly -- no toLegacyContact translation needed or correct here.
export async function archiveContact(
  id: string | number
): Promise<Contact> {
  const response = await apiFetch(
    `${API_BASE_URL}/contacts/${id}/archive`,
    {
      method: 'POST',
      headers: getAuthHeaders(),
    }
  );

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  return response.json();
}

// Unarchive a contact
export async function unarchiveContact(
  id: string | number
): Promise<Contact> {
  const response = await apiFetch(
    `${API_BASE_URL}/contacts/${id}/unarchive`,
    {
      method: 'POST',
      headers: getAuthHeaders(),
    }
  );

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  return response.json();
}
