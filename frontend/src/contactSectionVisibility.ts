// Shared "would this section render anything?" check for the contact detail
// page's General Information card (T30).
//
// A section subtitle must only render when the section actually has something
// to show: at least one field that is BOTH enabled in the user's field
// visibility settings AND carrying a value on this contact. The section body
// already had to reason about enabled-ness per field; hoisting the same check
// one level up means the header and the body never disagree, and a section
// with every field hidden or empty no longer leaves an orphan heading behind.
//
// `gender` is passed separately because it is a top-level record field (not a
// Card/CRMEnvelope member) — see ContactInformation.

import { Card, CRMEnvelope, getAnniversaryField } from './api/contacts';
import { ContactFieldKey } from './contactFields';

export type ContactSectionKey = 'about' | 'contact' | 'genderAndPronouns' | 'professional' | 'notes';

export const CONTACT_SECTION_FIELDS: Record<ContactSectionKey, ContactFieldKey[]> = {
  about: ['birthday', 'anniversary', 'anniversaries', 'personalInfo', 'keywords', 'preferredLanguages'],
  contact: ['phones', 'addresses', 'emails', 'socialProfiles', 'otherOnlineServices', 'imppAddresses', 'links'],
  genderAndPronouns: ['gender', 'speakToAs'],
  professional: ['organizations', 'titles', 'work_information'],
  notes: ['cardNotes', 'how_we_met', 'contact_information'],
};

function fieldHasValue(card: Card, crm: CRMEnvelope, gender: string | undefined, key: ContactFieldKey): boolean {
  switch (key) {
    case 'gender':
      return !!gender;
    case 'birthday':
      return !!getAnniversaryField(card.anniversaries, 'birth');
    case 'anniversary':
      return !!getAnniversaryField(card.anniversaries, 'wedding');
    case 'anniversaries':
      return !!card.anniversaries?.length;
    case 'personalInfo':
      return !!card.personalInfo?.length;
    case 'keywords':
      return !!card.keywords?.length;
    case 'preferredLanguages':
      return !!card.preferredLanguages?.length;
    case 'phones':
      return !!card.phones?.length;
    case 'addresses':
      return !!card.addresses?.length;
    case 'emails':
      return !!card.emails?.length;
    case 'socialProfiles':
      return !!card.socialProfiles?.length;
    case 'otherOnlineServices':
      return !!card.otherOnlineServices?.length;
    case 'imppAddresses':
      return !!card.imppAddresses?.length;
    case 'links':
      return !!card.links?.length;
    case 'organizations':
      return !!card.organizations?.length;
    case 'titles':
      return !!card.titles?.length;
    case 'work_information':
      return !!crm.work_information;
    case 'cardNotes':
      return !!card.notes?.length;
    case 'how_we_met':
      return !!crm.how_we_met;
    case 'contact_information':
      return !!crm.contact_information;
    case 'speakToAs': {
      const s = card.speakToAs;
      return !!s && (!!s.pronouns?.length || !!s.grammaticalGenders?.length);
    }
    default:
      return false;
  }
}

interface SectionOpts {
  card: Card;
  crm: CRMEnvelope;
  enabled: Set<ContactFieldKey>;
  gender?: string;
}

export function hasVisibleFields(section: ContactSectionKey, opts: SectionOpts): boolean {
  return CONTACT_SECTION_FIELDS[section].some(
    (key) => opts.enabled.has(key) && fieldHasValue(opts.card, opts.crm, opts.gender, key)
  );
}
