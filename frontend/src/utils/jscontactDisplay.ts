// Read-only display model for an RFC 9553 JSContact Card, used by the
// standing-share preview and recipient read view (ADR 0042, issue #2292).
//
// Allowlist on purpose: only the property -> section-token pairs below are ever
// rendered. A property this file does not name (an extension, an id, a future
// field) is dropped, so a field added to the projection later is not shown to a
// recipient until a person adds it here. The section tokens are the
// models.FieldSections tokens (frontend trap #4: keep in sync with
// EXPORT_FIELD_SECTIONS in api/export.ts).
import type { TFunction } from 'i18next';
import type { JSContactCard } from '../api/shareGrants';
import { type DateFormat, formatDateWithFormat } from '../DateFormatProvider';
import { humanizeToken } from './humanizeToken';

export interface JSContactSection {
  token: string;
  items: string[];
}

export interface JSContactDisplay {
  name: string;
  sections: JSContactSection[];
}

type Obj = Record<string, unknown>;

const isObj = (v: unknown): v is Obj => typeof v === 'object' && v !== null && !Array.isArray(v);
const str = (v: unknown): string => (typeof v === 'string' ? v.trim() : '');

function firstString(o: Obj, keys: string[]): string {
  for (const k of keys) {
    const s = str(o[k]);
    if (s) return s;
  }
  return '';
}

function withLabel(o: Obj, value: string): string {
  const label = str(o.label);
  return value && label ? `${label}: ${value}` : value;
}

function nameOf(raw: unknown): string {
  if (!isObj(raw)) return '';
  const full = str(raw.full);
  if (full) return full;
  if (!Array.isArray(raw.components)) return '';
  return raw.components
    .map((c) => (isObj(c) ? str(c.value) : ''))
    .filter(Boolean)
    .join(' ');
}

function addressOf(o: Obj): string {
  const full = str(o.full);
  if (full) return full;
  if (!Array.isArray(o.components)) return '';
  return o.components
    .map((c) => (isObj(c) ? str(c.value) : ''))
    .filter(Boolean)
    .join(', ');
}

function anniversaryDate(o: Obj): string {
  const d = o.date;
  if (!isObj(d)) return '';
  const utc = str(d.utc);
  if (utc) return utc;
  const part = (v: unknown) => (typeof v === 'number' && v > 0 ? v : 0);
  const year = part(d.year);
  const month = part(d.month);
  const day = part(d.day);
  // formatDateWithFormat reads a date-only string digit for digit; pad the
  // month/day so a partial date matches its YYYY-MM-DD / --MM-DD shapes.
  if (year && month && day) {
    return `${String(year).padStart(4, '0')}-${String(month).padStart(2, '0')}-${String(day).padStart(2, '0')}`;
  }
  if (month && day) return `--${String(month).padStart(2, '0')}-${String(day).padStart(2, '0')}`;
  return [year, month, day].filter(Boolean).join('-');
}

// Anniversary kinds are a backend enum; a kind without a translation (a
// future token) falls back to the raw token rather than disappearing.
function anniversaryOf(o: Obj, t: TFunction, dateFormat: DateFormat): string {
  const kind = str(o.kind);
  const date = anniversaryDate(o);
  const label = kind ? t(`contacts.anniversaryFields.kindOptions.${kind}`, kind) : '';
  const formatted = date ? formatDateWithFormat(date, dateFormat) : '';
  return label && formatted ? `${label}: ${formatted}` : formatted;
}

// relatedTo.relation keys are the RFC 6350 section 6.6.6 RELATED TYPE registry
// (issue #3014), which the backend passes through unvalidated: a registry key
// gets a translation, any other (vendor) key reads as humanised words.
function relatedOf(o: Obj, t: TFunction): string {
  if (!isObj(o.relation)) return '';
  return Object.keys(o.relation)
    .map((key) => {
      const token = key.toLowerCase();
      return t(`standingShares.relatedToRelations.${token}`, humanizeToken(key));
    })
    .join(', ');
}

function pronounsOf(o: Obj): string {
  return str(o.pronouns);
}

type Summariser = (entry: Obj, t: TFunction, dateFormat: DateFormat) => string;

// Map-of-objects properties: JSContact property -> [section token, summariser].
const OBJECT_MAPS: Record<string, [string, Summariser]> = {
  emails: ['emails', (o) => withLabel(o, str(o.address))],
  phones: ['phones', (o) => withLabel(o, str(o.number))],
  addresses: ['addresses', (o) => withLabel(o, addressOf(o))],
  organizations: ['organizations', (o) => str(o.name)],
  anniversaries: ['anniversaries', anniversaryOf],
  media: ['media', (o) => str(o.uri)],
  onlineServices: [
    'online_services',
    (o) => {
      const service = str(o.service);
      const who = firstString(o, ['user', 'uri']);
      return service && who ? `${service}: ${who}` : service || who;
    },
  ],
  links: ['links', (o) => withLabel(o, str(o.uri))],
  notes: ['notes', (o) => str(o.note)],
  relatedTo: ['related_to', relatedOf],
  personalInfo: [
    'personal_info',
    (o) => {
      const kind = str(o.kind);
      const value = str(o.value);
      return kind && value ? `${kind}: ${value}` : value;
    },
  ],
  preferredLanguages: ['languages', (o) => str(o.language)],
};

// Render order mirrors the section picker.
const ORDER = [
  'emails',
  'phones',
  'addresses',
  'organizations',
  'anniversaries',
  'media',
  'online_services',
  'links',
  'notes',
  'keywords',
  'related_to',
  'personal_info',
  'speak_to_as',
  'languages',
];

export function describeJSContact(
  card: JSContactCard,
  t: TFunction,
  dateFormat: DateFormat,
): JSContactDisplay {
  const byToken = new Map<string, string[]>();
  const add = (token: string, items: string[]) => {
    const kept = items.filter(Boolean);
    if (kept.length > 0) byToken.set(token, [...(byToken.get(token) ?? []), ...kept]);
  };

  for (const [prop, [token, summarise]] of Object.entries(OBJECT_MAPS)) {
    const map = card[prop];
    if (!isObj(map)) continue;
    add(
      token,
      Object.values(map).map((e) => (isObj(e) ? summarise(e, t, dateFormat) : '')),
    );
  }

  if (isObj(card.keywords)) add('keywords', Object.keys(card.keywords));
  if (isObj(card.speakToAs)) {
    const pronouns = card.speakToAs.pronouns;
    if (isObj(pronouns)) {
      add(
        'speak_to_as',
        Object.values(pronouns).map((p) => (isObj(p) ? pronounsOf(p) : '')),
      );
    }
  }

  return {
    name: nameOf(card.name),
    sections: ORDER.filter((token) => byToken.has(token)).map((token) => ({
      token,
      items: byToken.get(token) ?? [],
    })),
  };
}
