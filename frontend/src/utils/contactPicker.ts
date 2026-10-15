// W3C Contact Picker import (issue #164). `navigator.contacts.select()` hands a
// PWA the contacts the user picks, mediated by the browser (no READ_CONTACTS
// grant). Chrome on Android only. The picked contacts are turned into a vCard
// 3.0 file so they flow through the existing VCF import wizard -- the same
// parser, duplicate preview and merge actions as a file upload.

type PickerProperty = 'name' | 'email' | 'tel' | 'address' | 'icon';

interface PickerAddress {
  addressLine?: string[];
  city?: string;
  region?: string;
  postalCode?: string;
  country?: string;
}

export interface PickedContact {
  name?: string[];
  email?: string[];
  tel?: string[];
  address?: PickerAddress[];
  icon?: Blob[];
}

interface ContactsManagerLike {
  getProperties(): Promise<string[]>;
  select(properties: PickerProperty[], options?: { multiple?: boolean }): Promise<PickedContact[]>;
}

const WANTED: PickerProperty[] = ['name', 'email', 'tel', 'address', 'icon'];

function manager(): ContactsManagerLike | null {
  if (typeof navigator === 'undefined' || typeof window === 'undefined') return null;
  if (!('contacts' in navigator) || !('ContactsManager' in window)) return null;
  return (navigator as unknown as { contacts: ContactsManagerLike }).contacts;
}

export function isContactPickerSupported(): boolean {
  return manager() !== null;
}

// vCard TEXT escaping (RFC 6350 §3.4): backslash, comma, semicolon, newline.
function esc(v: string): string {
  return v
    .replace(/\\/g, '\\\\')
    .replace(/\n|\r\n?/g, '\\n')
    .replace(/,/g, '\\,')
    .replace(/;/g, String.raw`\;`);
}

// The picker returns a single display string, with no structured name parts.
// Split it the way a person would read it: the last word is the family name.
function structuredName(display: string): { family: string; given: string } {
  const words = display.trim().split(/\s+/);
  if (words.length < 2) return { family: '', given: words[0] ?? '' };
  return { family: words[words.length - 1], given: words.slice(0, -1).join(' ') };
}

// Chrome on Android fills `addressLine` with the whole formatted address
// ("451 Oak Grove Ave, South Beloit, IL 61080, USA") on top of the separate
// city/region/postalCode/country fields, which would leave the street line
// carrying the locality twice. Peel those trailing parts back off.
function streetLine(a: PickerAddress): string {
  let line = (a.addressLine ?? []).filter(Boolean).join(', ').trim();
  for (const part of [a.country, a.postalCode, a.region, a.city]) {
    const tail = part?.trim().toLowerCase();
    if (!tail || !line.toLowerCase().endsWith(tail)) continue;
    const head = line.slice(0, line.length - tail.length);
    // Only a whole trailing component: it must follow a space or comma.
    if (!/[\s,]$/.test(head)) continue;
    const trimmed = head.replace(/[\s,]+$/, '');
    if (trimmed !== '') line = trimmed;
  }
  return line;
}

// The picker repeats a value in several spellings ("262-909-6849" and
// "2629096849"); keep the first of each digit string / lowercased email.
function dedupe(values: string[] | undefined, key: (v: string) => string): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const raw of values ?? []) {
    const v = raw.trim();
    const k = key(v);
    if (v === '' || k === '' || seen.has(k)) continue;
    seen.add(k);
    out.push(v);
  }
  return out;
}

function toBase64(bytes: Uint8Array): string {
  let bin = '';
  for (let i = 0; i < bytes.length; i += 0x8000) {
    bin += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
  }
  return btoa(bin);
}

async function photoLine(icon: Blob | undefined): Promise<string | null> {
  if (!icon || icon.size === 0) return null;
  const type = /^image\/(png|jpeg|gif|webp)$/.exec(icon.type)?.[1];
  const bytes = new Uint8Array(await icon.arrayBuffer());
  return `PHOTO;ENCODING=b;TYPE=${(type ?? 'jpeg').toUpperCase()}:${toBase64(bytes)}`;
}

export async function contactsToVCard(contacts: PickedContact[]): Promise<string> {
  const cards: string[] = [];
  for (const c of contacts) {
    const name = c.name?.find((n) => n.trim() !== '')?.trim();
    const emails = dedupe(c.email, (e) => e.toLowerCase());
    const tels = dedupe(c.tel, (t) => t.replace(/\D/g, '') || t);
    const addrs = (c.address ?? []).filter(
      (a) => (a.addressLine ?? []).some(Boolean) || a.city || a.region || a.postalCode || a.country,
    );
    // Nothing the importer could key a contact on.
    if (!name && emails.length === 0 && tels.length === 0) continue;

    const lines = ['BEGIN:VCARD', 'VERSION:3.0'];
    const display = name ?? emails[0] ?? tels[0];
    lines.push(`FN:${esc(display)}`);
    const { family, given } = name ? structuredName(name) : { family: '', given: '' };
    lines.push(`N:${esc(family)};${esc(given)};;;`);
    for (const e of emails) lines.push(`EMAIL:${esc(e)}`);
    for (const t of tels) lines.push(`TEL:${esc(t)}`);
    for (const a of addrs) {
      lines.push(
        `ADR:;;${esc(streetLine(a))};${esc(a.city ?? '')};${esc(a.region ?? '')};${esc(a.postalCode ?? '')};${esc(a.country ?? '')}`,
      );
    }
    const photo = await photoLine(c.icon?.[0]);
    if (photo) lines.push(photo);
    lines.push('END:VCARD');
    cards.push(lines.join('\r\n'));
  }
  return cards.length === 0 ? '' : `${cards.join('\r\n')}\r\n`;
}

// Opens the browser's contact picker (must be called from a user gesture) and
// returns the selection as a .vcf File, or null when the user picked nothing or
// cancelled. Properties the device does not support are not requested.
export async function pickContactsAsVCardFile(): Promise<File | null> {
  const mgr = manager();
  if (!mgr) return null;
  const supported = await mgr.getProperties();
  const props = WANTED.filter((p) => supported.includes(p));
  let picked: PickedContact[];
  try {
    picked = await mgr.select(props, { multiple: true });
  } catch (err) {
    // Dismissing the picker rejects with an AbortError; that is not a failure.
    if (err instanceof DOMException && err.name === 'AbortError') return null;
    throw err;
  }
  const vcf = await contactsToVCard(picked);
  if (vcf === '') return null;
  return new File([vcf], 'device-contacts.vcf', { type: 'text/vcard' });
}
