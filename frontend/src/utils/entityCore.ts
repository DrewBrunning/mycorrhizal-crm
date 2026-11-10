// Entity `core` <-> form helpers (issue #2301, ADR 0043 decision 1).
//
// The per-kind core JSON is validated by typed Go structs
// (backend/models/entity.go: OrganisationCore, PlaceCore, PetCore) with
// DisallowUnknownFields, so these helpers only ever emit the keys the backend
// knows, and carry through (never drop) whatever an existing core holds.
import type { Entity, EntityKind } from '../api/entities';
import type { PartialDate } from '../api/lifeEvents';

type Json = Record<string, unknown>;
type CoreComponent = { kind: string; value: string };

export interface OrganisationForm {
  sortAs: string;
  unitsText: string;
  url: string;
  notes: string;
}
export interface PlaceForm {
  street: string;
  city: string;
  region: string;
  postal: string;
  country: string;
  notes: string;
}
export interface PetForm {
  species: string;
  breed: string;
  birth: string;
  death: string;
  notes: string;
}
export type EntityForm = OrganisationForm | PlaceForm | PetForm;

const asObject = (v: unknown): Json => (v && typeof v === 'object' ? (v as Json) : {});
const asString = (v: unknown): string => (typeof v === 'string' ? v : '');

// -- partial dates ----------------------------------------------------------

/** 'YYYY', 'YYYY-MM' or 'YYYY-MM-DD' -> PartialDate; blank -> undefined. */
export function parsePartialInput(raw: string): PartialDate | undefined | 'invalid' {
  const s = raw.trim();
  if (!s) return undefined;
  const parts = s.split('-');
  const [y, mo, d] = parts;
  if (
    parts.length > 3 ||
    !/^\d{4}$/.test(y) ||
    (mo !== undefined && !/^\d{2}$/.test(mo)) ||
    (d !== undefined && !/^\d{2}$/.test(d))
  ) {
    return 'invalid';
  }
  const year = Number(y);
  const month = mo !== undefined ? Number(mo) : undefined;
  const day = d !== undefined ? Number(d) : undefined;
  if (month !== undefined && (month < 1 || month > 12)) return 'invalid';
  if (day !== undefined && month !== undefined) {
    if (day < 1 || day > new Date(Date.UTC(year, month, 0)).getUTCDate()) return 'invalid';
  }
  return { year, ...(month !== undefined ? { month } : {}), ...(day !== undefined ? { day } : {}) };
}

export function formatPartialInput(d: PartialDate | undefined | null): string {
  if (!d || d.year == null) return '';
  const pad = (n: number) => String(n).padStart(2, '0');
  let out = String(d.year);
  if (d.month != null) {
    out += `-${pad(d.month)}`;
    if (d.day != null) out += `-${pad(d.day)}`;
  }
  return out;
}

// -- core <-> form ----------------------------------------------------------

export function coreToForm(kind: EntityKind, core: Json | undefined): EntityForm {
  const c = asObject(core);
  if (kind === 'organisation') {
    const org = asObject(c.organization);
    const units = Array.isArray(org.units) ? org.units : [];
    return {
      sortAs: asString(org.sortAs),
      unitsText: units.map((u) => asString(asObject(u).name)).join('\n'),
      url: asString(c.url),
      notes: asString(c.notes),
    };
  }
  if (kind === 'place') {
    const comps = componentsOf(c.address);
    const find = (k: string) => comps.find((x) => x.kind === k)?.value ?? '';
    return {
      street: find('name'),
      city: find('locality'),
      region: find('region'),
      postal: find('postcode'),
      country: find('country'),
      notes: asString(c.notes),
    };
  }
  return {
    species: asString(c.species),
    breed: asString(c.breed),
    birth: formatPartialInput(c.birth as PartialDate | undefined),
    death: formatPartialInput(c.death as PartialDate | undefined),
    notes: asString(c.notes),
  };
}

function componentsOf(address: unknown): CoreComponent[] {
  const comps = asObject(address).components;
  return Array.isArray(comps) ? (comps as CoreComponent[]) : [];
}

const PLACE_FORM_KINDS = new Set(['name', 'locality', 'region', 'postcode', 'country']);

/**
 * Build the core to PUT/POST. `existing` is the stored core: keys the form does
 * not edit (an organisation id, a place's coordinates and other address
 * components, ...) are carried through unchanged. Throws Error(`<field>`) for an
 * unparsable pet date so the caller can attach it to the right input.
 */
export function formToCore(
  kind: EntityKind,
  name: string,
  form: EntityForm,
  existing?: Json,
): Json {
  const base = { ...asObject(existing) };
  const put = (obj: Json, key: string, value: unknown) => {
    if (value === '' || value === undefined || (Array.isArray(value) && value.length === 0)) {
      delete obj[key];
    } else {
      obj[key] = value;
    }
  };

  if (kind === 'organisation') {
    const f = form as OrganisationForm;
    const org = { ...asObject(base.organization) };
    const prior = Array.isArray(org.units) ? (org.units as Json[]) : [];
    const units = f.unitsText
      .split('\n')
      .map((l) => l.trim())
      .filter(Boolean)
      .map((line, i) => (prior[i]?.name === line ? prior[i] : { name: line }));
    org.name = name;
    put(org, 'sortAs', f.sortAs.trim());
    put(org, 'units', units);
    base.organization = org;
    put(base, 'url', f.url.trim());
    put(base, 'notes', f.notes.trim());
    return base;
  }

  if (kind === 'place') {
    const f = form as PlaceForm;
    const address = { ...asObject(base.address) };
    const kept = componentsOf(address).filter((c) => !PLACE_FORM_KINDS.has(c.kind));
    const mapped: CoreComponent[] = [
      { kind: 'name', value: f.street.trim() },
      ...kept,
      { kind: 'locality', value: f.city.trim() },
      { kind: 'region', value: f.region.trim() },
      { kind: 'postcode', value: f.postal.trim() },
      { kind: 'country', value: f.country.trim() },
    ].filter((c) => c.value !== '');
    put(address, 'components', mapped);
    base.address = address;
    put(base, 'notes', f.notes.trim());
    return base;
  }

  const f = form as PetForm;
  put(base, 'species', f.species.trim());
  put(base, 'breed', f.breed.trim());
  for (const field of ['birth', 'death'] as const) {
    const parsed = parsePartialInput(f[field]);
    if (parsed === 'invalid') throw new Error(field);
    put(base, field, parsed);
  }
  put(base, 'notes', f.notes.trim());
  return base;
}

/** One short secondary line for a list row. */
export function coreSummary(entity: Pick<Entity, 'kind' | 'core'>): string {
  const c = asObject(entity.core);
  if (entity.kind === 'organisation') return asString(c.url);
  if (entity.kind === 'place') {
    const comps = componentsOf(c.address);
    const find = (k: string) => comps.find((x) => x.kind === k)?.value ?? '';
    return [find('locality'), find('country')].filter(Boolean).join(', ');
  }
  return [asString(c.species), asString(c.breed)].filter(Boolean).join(', ');
}

// -- merge ------------------------------------------------------------------

// Mirrors backend/services/entity_merge.go#entityFillableFields (frontend trap
// #4: keep in sync by hand). These are the core fields a merge resolution may
// fill from the loser; the backend rejects any other key.
const FILLABLE: Record<EntityKind, string[]> = {
  organisation: ['organization.sortAs', 'organization.units', 'url', 'notes'],
  place: ['address', 'notes'],
  pet: ['species', 'breed', 'birth', 'death', 'notes'],
};

function fieldValue(core: Json, field: string): unknown {
  if (field.startsWith('organization.')) return asObject(core.organization)[field.slice(13)];
  return core[field];
}

function isEmptyValue(field: string, v: unknown): boolean {
  if (v == null || v === '') return true;
  if (Array.isArray(v)) return v.length === 0;
  if (field === 'address') {
    const a = asObject(v);
    return componentsOf(a).length === 0 && !asString(a.full);
  }
  if (typeof v === 'object') {
    const d = v as PartialDate;
    return d.year == null && d.month == null && d.day == null;
  }
  return false;
}

/** Fields a merge may fill: empty on the survivor, non-empty on the loser. */
export function fillableMergeFields(survivor: Entity, loser: Entity): string[] {
  const s = asObject(survivor.core);
  const l = asObject(loser.core);
  return FILLABLE[survivor.kind].filter(
    (f) => isEmptyValue(f, fieldValue(s, f)) && !isEmptyValue(f, fieldValue(l, f)),
  );
}
