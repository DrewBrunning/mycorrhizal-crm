// Curated icon slugs for LinkFieldType.icon (T34) — a fixed dropdown of MDI
// path names for the defaults this registry seeds with (mirroring
// backend/models/link_field_type.go's LinkFieldTypeDefaults icon choices
// exactly; keep the two in sync by hand — no dynamic icon-list endpoint
// exists, same convention as every other frontend enum mirror in this repo),
// plus free-text entry for anything a user adds that isn't in the list.
// Unrecognized/missing slugs fall back to a generic link icon rather than
// rendering nothing.
import {
  mdiMessageLock,
  mdiFacebookMessenger,
  mdiWhatsapp,
  mdiSend,
  mdiChatOutline,
  mdiMatrix,
  mdiSlack,
  mdiTwitter,
  mdiButterflyOutline,
  mdiInstagram,
  mdiAt,
  mdiMusicNote,
  mdiReddit,
  mdiSpotify,
  mdiTwitch,
  mdiLinkVariant,
} from '@mdi/js';

export const LINK_FIELD_TYPE_ICONS: Record<string, string> = {
  mdiMessageLock,
  mdiFacebookMessenger,
  mdiWhatsapp,
  mdiSend,
  mdiChatOutline,
  mdiMatrix,
  mdiSlack,
  mdiTwitter,
  mdiButterflyOutline,
  mdiInstagram,
  mdiAt,
  mdiMusicNote,
  mdiReddit,
  mdiSpotify,
  mdiTwitch,
};

export const LINK_FIELD_TYPE_ICON_FALLBACK = mdiLinkVariant;

export const LINK_FIELD_TYPE_ICON_OPTIONS = Object.keys(LINK_FIELD_TYPE_ICONS);

export function resolveLinkFieldTypeIcon(icon: string | undefined): string {
  if (!icon) return LINK_FIELD_TYPE_ICON_FALLBACK;
  return LINK_FIELD_TYPE_ICONS[icon] || LINK_FIELD_TYPE_ICON_FALLBACK;
}
