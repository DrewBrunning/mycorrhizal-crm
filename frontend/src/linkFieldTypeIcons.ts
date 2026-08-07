// Curated icon slugs for LinkFieldType.icon (T34) — a hand-maintained list of
// MDI path names for the defaults this registry seeds with (mirroring
// backend/models/link_field_type.go's LinkFieldTypeDefaults icon choices
// exactly; keep the two in sync by hand — no dynamic icon-list endpoint
// exists, same convention as every other frontend enum mirror in this repo).
// This list is used only for the Settings Autocomplete's curated suggestion
// dropdown; resolveLinkFieldTypeIcon resolves against the full @mdi/js export
// surface, so any valid MDI name typed into the free-text field renders
// correctly (T43).
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
import * as mdiIcons from '@mdi/js';

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
  const path = (mdiIcons as Record<string, string>)[icon];
  return path && typeof path === 'string' ? path : LINK_FIELD_TYPE_ICON_FALLBACK;
}
