import { useMemo, useEffect, useState, ReactNode } from 'react';
import { Card, CardContent, Divider, Stack, Box, Typography, SvgIcon, IconButton, Tooltip, Collapse, Button } from '@mui/material';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import EmailIcon from '@mui/icons-material/Email';
import PhoneIcon from '@mui/icons-material/Phone';
import SmsOutlinedIcon from '@mui/icons-material/SmsOutlined';
import CakeIcon from '@mui/icons-material/Cake';
import CelebrationIcon from '@mui/icons-material/Celebration';
import HomeIcon from '@mui/icons-material/Home';
import WorkIcon from '@mui/icons-material/Work';
import BusinessIcon from '@mui/icons-material/Business';
import BadgeIcon from '@mui/icons-material/Badge';
import LanguageIcon from '@mui/icons-material/Language';
import ChatIcon from '@mui/icons-material/Chat';
import PersonOutlineIcon from '@mui/icons-material/PersonOutline';
import { mdiNoteMultipleOutline } from '@mdi/js';
import PeopleIcon from '@mui/icons-material/People';
import StarIcon from '@mui/icons-material/Star';
import { useTranslation } from 'react-i18next';
import EditableField from './EditableField';
import EditableArrayField from './EditableArrayField';
import MultiValueField from './MultiValueField';
import AddressFields from './AddressFields';
import CustomFieldValueRow from './CustomFieldValueRow';
import { FieldDefinition } from '../api/fieldDefinitions';
import {
  Card as CardModel,
  CRMEnvelope,
  ContactValue,
  ContactAddress,
  CardOnlineService,
  CardPersonalInfo,
  CardNote,
  CardLanguagePref,
  CardSpeakToAs,
  CardAnniversary,
  cardEmailsToValues,
  valuesToCardEmails,
  cardPhonesToValues,
  valuesToCardPhones,
  cardLinksToValues,
  valuesToCardLinks,
  cardAddressesToValues,
  valuesToCardAddresses,
  getAnniversaryField,
  formatAnniversaryDate,
  getOrganizationFields,
  getTitleField,
} from '../api/contacts';
import { ContactFieldKey, resolveEnabledFields, GENDER_OPTIONS } from '../contactFields';
import { hasVisibleFields } from '../contactSectionVisibility';
import { hasImportedResources } from './ImportedResourcesSection';
import { hasRelatedToOrMembers } from './RelatedToMembersSection';
import { useDateFormat } from '../DateFormatProvider';
import OnlineServiceEditor from './OnlineServiceEditor';
import SpeakToAsEditor from './SpeakToAsEditor';
import PersonalInfoEditor from './PersonalInfoEditor';
import KeywordsEditor from './KeywordsEditor';
import CardNotesEditor from './CardNotesEditor';
import PreferredLanguagesEditor from './PreferredLanguagesEditor';
import AnniversariesEditor from './AnniversariesEditor';
import ImportedResourcesSection from './ImportedResourcesSection';
import RelatedToMembersSection from './RelatedToMembersSection';
import CopyButton from './CopyButton';
import { useLinkFieldTypes } from '../hooks/useLinkFieldTypes';
import {
  buildTelLink,
  buildSmsLink,
  buildMailtoLink,
  buildAddressLink,
  formatAddressLine,
  resolveOnlineServiceLink,
  isSafeUrlString,
  looksLikeAbsoluteUri,
} from '../utils/linkResolution';
import { resolveLinkFieldTypeIcon } from '../linkFieldTypeIcons';

interface ContactInformationProps {
  card: CardModel;
  crm: CRMEnvelope;
  gender?: string;
  editingField: string | null;
  editValue: string;
  validationError: string;
  onEditStart: (field: string, value: string) => void;
  onEditCancel: () => void;
  onEditSave: (field: string) => void;
  onEditValueChange: (value: string) => void;
  onUpdateCard: (patch: Partial<CardModel>) => Promise<void>;
  enabledFields?: Set<ContactFieldKey>;
  // Custom fields (v2, T6/T7)
  fieldDefinitions?: FieldDefinition[];
  fieldValuesByDefinition?: Map<string, unknown>;
  onSaveFieldValue?: (definitionId: string, value: unknown) => Promise<void>;
}

const iconSx = { mr: 1, color: 'text.secondary', fontSize: '1.2rem' };
const cloneValues = <T extends object>(v: T[]): T[] => v.map((x) => ({ ...x }));

// Small section headings for the General Info tab, matching the caption-style
// group labels the header uses for circles/tags.
// T74: always spans both of Level 1's grid columns at lg+ -- a heading
// belongs above the fields it introduces, not squeezed into one column
// beside them. Harmless below lg, where the parent isn't a grid.
const SectionHeading = ({ label }: { label: string }) => (
  <Box sx={{ pt: 1, gridColumn: { lg: '1 / -1' } }}>
    <Typography variant="overline" color="text.secondary" sx={{ letterSpacing: 0.08, fontSize: '0.72rem' }}>
      {label}
    </Typography>
    <Divider sx={{ mt: 0.5 }} />
  </Box>
);

// T74 Level 1: wraps a field whose editor is too wide/tall to share a
// ~530px column with its neighbor (multi-line text, SpeakToAs' two lists,
// card notes) so it spans both grid columns instead of narrowing.
const FullSpanField = ({ children }: { children: ReactNode }) => (
  <Box sx={{ gridColumn: { lg: '1 / -1' } }}>{children}</Box>
);

export default function ContactInformation({
  card,
  crm,
  gender = '',
  editingField,
  editValue,
  validationError,
  onEditStart,
  onEditCancel,
  onEditSave,
  onEditValueChange,
  onUpdateCard,
  enabledFields,
  fieldDefinitions = [],
  fieldValuesByDefinition,
  onSaveFieldValue,
}: ContactInformationProps) {
  const { t } = useTranslation();
  const { formatBirthday, getBirthdayPlaceholder, calculateAge } = useDateFormat();
  const enabled = enabledFields ?? resolveEnabledFields(null);
  const isOn = (key: ContactFieldKey) => enabled.has(key);
  // Read-only fetch of the user's LinkFieldType registry (T34) — resolves
  // socialProfiles/otherOnlineServices/imppAddresses handles to tappable
  // links and, since T44, feeds the service-name Autocomplete in all three
  // fields' editors. Only fetched when one of those fields is actually
  // enabled: ListLinkFieldTypes does a write (lazy-seeds 19 defaults) on a
  // user's first call, and none of the three is in
  // DEFAULT_ENABLED_CONTACT_FIELDS, so an unconditional fetch would cost
  // every contact-page view for accounts that never touch this registry.
  const registryFieldsEnabled =
    isOn('socialProfiles') || isOn('otherOnlineServices') || isOn('imppAddresses');
  const { linkFieldTypes, refreshLinkFieldTypes } = useLinkFieldTypes();
  useEffect(() => {
    if (registryFieldsEnabled) refreshLinkFieldTypes();
  }, [registryFieldsEnabled, refreshLinkFieldTypes]);

  const birthday = getAnniversaryField(card.anniversaries, 'birth') || '';
  const anniversary = getAnniversaryField(card.anniversaries, 'wedding') || '';
  const { organization = '', department = '' } = getOrganizationFields(card.organizations);
  const jobTitle = getTitleField(card.titles, 'title') || '';
  const role = getTitleField(card.titles, 'role') || '';

  const birthdayAgeSuffix = useMemo(() => {
    if (!birthday) return undefined;
    const age = calculateAge(birthday);
    if (age === null) return undefined;
    return t('dashboard.yearsOld', { age });
  }, [birthday, t, calculateAge]);

  // Logical groupings for the General Info tab (field-ordering pass). A heading
  // is only shown when at least one of its fields is both enabled AND present
  // on this contact (T30) — a section with every field hidden or empty must not
  // leave an orphan heading behind.
  const showAbout = hasVisibleFields('about', { card, crm, enabled, gender });
  const showContact = hasVisibleFields('contact', { card, crm, enabled, gender });
  const showAddressThem = hasVisibleFields('genderAndPronouns', { card, crm, enabled, gender });
  const showProfessional = hasVisibleFields('professional', { card, crm, enabled, gender });
  const showNotes = hasVisibleFields('notes', { card, crm, enabled, gender });
  // Card metadata isn't a toggleable field — it renders the read-only imported
  // resource / related-entity sections, which themselves render nothing when
  // the card has none. Share that check so the heading doesn't appear over an
  // empty card (T30's original report: "Card Metadata showing when all fields
  // are hidden").
  const showMetadata = hasImportedResources(card) || hasRelatedToOrMembers(card);
  // Imported-resource lists (media, cryptoKeys, relatedTo, members, ...) can
  // grow arbitrarily large (a vCard can carry hundreds of entries), so the
  // whole section collapses behind its heading by default (v0.4.1). The data
  // is never lost — it just takes a click to reveal.
  const [metadataExpanded, setMetadataExpanded] = useState(false);

  // Gender is a top-level record field (not part of the Card/CRM envelope), so
  // it is passed in separately and shown here next to pronouns/grammatical
  // gender, where it conceptually belongs.
  const genderDisplay = GENDER_OPTIONS.includes(gender as (typeof GENDER_OPTIONS)[number])
    ? t(`contacts.${gender}`)
    : gender;

  // Every displayed field value gets a copy button (T34), tappable or not —
  // the ask is explicit that every field gets it, even ones with no other
  // action. Single-action fields (email, address, online-service handles,
  // raw links) become the tappable value itself, an <a href> around the
  // displayed text; phone is the only multi-action field (call vs text), so
  // it gets discrete icon buttons beside plain text instead of ambiguously
  // wrapping the whole value in one link.
  // A phone's TYPE tokens can carry more than one feature (vCard TYPE=VOICE,CELL
  // is a real, common iOS/Google export shape) -- `r.type` alone is only the
  // FIRST token (cardPhonesToValues' `features?.[0] || contexts?.[0]`), so a
  // cell number imported as "voice,cell" would otherwise show as a plain
  // landline with no text button, and a "voice,fax" number would wrongly
  // gain a call button. Check the full features/contexts arrays, falling
  // back to `type` only for rows that carry neither (e.g. minimal manually-
  // entered data where only a bare type string was ever set).
  const phoneHasToken = (r: ContactValue, token: string): boolean =>
    r.features?.includes(token) || r.contexts?.includes(token) || r.type === token || false;

  const renderPhoneList = (rows: ContactValue[] | undefined) => {
    if (!rows || rows.length === 0) return <Typography variant="body2" color="text.disabled">—</Typography>;
    return (
      <Stack spacing={0.25}>
        {rows.map((r, i) => {
          const isFax = phoneHasToken(r, 'fax');
          const isCell = !isFax && phoneHasToken(r, 'cell');
          return (
            <Box key={i} sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 1 }}>
              <Box sx={{ minWidth: 0, overflowWrap: 'anywhere' }}>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.25 }}>
                  {r.pref === 1 && (
                    <StarIcon sx={{ fontSize: '0.85rem', color: 'primary.main', flexShrink: 0 }} />
                  )}
                  {!isFax ? (
                    <Typography variant="body2" component="a" href={buildTelLink(r.value)} sx={{ color: 'inherit' }}>
                      {r.value}
                      {r.type ? ` (${t(`contacts.types.${r.type}`, r.type)})` : ''}
                    </Typography>
                  ) : (
                    <Typography variant="body2">
                      {r.value}
                      {r.type ? ` (${t(`contacts.types.${r.type}`, r.type)})` : ''}
                    </Typography>
                  )}
                </Box>
              </Box>
              <Box sx={{ flexShrink: 0, display: 'flex', alignItems: 'center', gap: 0.25 }}>
                {!isFax && (
                  <Tooltip title={t('contactDetail.call')}>
                    <IconButton size="small" color="primary" component="a" href={buildTelLink(r.value)} aria-label={`${t('contactDetail.call')} ${r.value}`}>
                      <PhoneIcon fontSize="inherit" />
                    </IconButton>
                  </Tooltip>
                )}
                {isCell && (
                  <Tooltip title={t('contactDetail.text')}>
                    <IconButton size="small" color="primary" component="a" href={buildSmsLink(r.value)} aria-label={`${t('contactDetail.text')} ${r.value}`}>
                      <SmsOutlinedIcon fontSize="inherit" />
                    </IconButton>
                  </Tooltip>
                )}
                <CopyButton value={r.value} label={`${t('contactDetail.phone')} ${r.value}`} />
              </Box>
            </Box>
          );
        })}
      </Stack>
    );
  };

  const renderEmailList = (rows: ContactValue[] | undefined) => {
    if (!rows || rows.length === 0) return <Typography variant="body2" color="text.disabled">—</Typography>;
    return (
      <Stack spacing={0.25}>
        {rows.map((r, i) => (
          <Box key={i} sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 1 }}>
            <Box sx={{ minWidth: 0, overflowWrap: 'anywhere' }}>
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.25 }}>
                {r.pref === 1 && (
                  <StarIcon sx={{ fontSize: '0.85rem', color: 'primary.main', flexShrink: 0 }} />
                )}
                <Typography variant="body2" component="a" href={buildMailtoLink(r.value)} sx={{ color: 'inherit' }}>
                  {r.value}
                  {r.type ? ` (${t(`contacts.types.${r.type}`, r.type)})` : ''}
                </Typography>
              </Box>
            </Box>
            <Box sx={{ flexShrink: 0, display: 'flex', alignItems: 'center', gap: 0.25 }}>
              <CopyButton value={r.value} label={`${t('contactDetail.email')} ${r.value}`} />
            </Box>
          </Box>
        ))}
      </Stack>
    );
  };

  // Shared by Card.Links (already full URLs by definition) and IMPP
  // addresses (uri-shaped, per T29): links directly if the value is both a
  // recognized absolute URI and passes the safe-scheme guard, else falls
  // back to plain text — always with a copy button.
  const renderUriValueList = (rows: ContactValue[] | undefined, copyLabel: string) => {
    if (!rows || rows.length === 0) return <Typography variant="body2" color="text.disabled">—</Typography>;
    return (
      <Stack spacing={0.25}>
        {rows.map((r, i) => {
          const tappable = looksLikeAbsoluteUri(r.value) && isSafeUrlString(r.value);
          return (
            <Box key={i} sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 1 }}>
              <Box sx={{ minWidth: 0, overflowWrap: 'anywhere' }}>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.25 }}>
                  {r.pref === 1 && (
                    <StarIcon sx={{ fontSize: '0.85rem', color: 'primary.main', flexShrink: 0 }} />
                  )}
                  {tappable ? (
                    <Typography
                      variant="body2"
                      component="a"
                      href={r.value}
                      target="_blank"
                      rel="noopener noreferrer"
                      sx={{ color: 'inherit', wordBreak: 'break-all' }}
                    >
                      {r.value}
                      {r.type ? ` (${t(`contacts.types.${r.type}`, r.type)})` : ''}
                    </Typography>
                  ) : (
                    <Typography variant="body2" sx={{ wordBreak: 'break-all' }}>
                      {r.value}
                      {r.type ? ` (${t(`contacts.types.${r.type}`, r.type)})` : ''}
                    </Typography>
                  )}
                </Box>
              </Box>
              <Box sx={{ flexShrink: 0, display: 'flex', alignItems: 'center', gap: 0.25 }}>
                {r.value && <CopyButton value={r.value} label={`${copyLabel} ${r.value}`} />}
              </Box>
            </Box>
          );
        })}
      </Stack>
    );
  };

  const renderAddressList = (rows: ContactAddress[] | undefined) => {
    if (!rows || rows.length === 0) return <Typography variant="body2" color="text.disabled">—</Typography>;
    return (
      <Stack spacing={0.25}>
        {rows.map((a, i) => {
          const formatted = formatAddressLine(a);
          const href = buildAddressLink(a);
          const suffix = a.type ? ` (${t(`contacts.types.${a.type}`, a.type)})` : '';
          return (
            <Box key={i} sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 1 }}>
              <Box sx={{ minWidth: 0, overflowWrap: 'anywhere' }}>
                {href ? (
                  <Typography variant="body2" component="a" href={href} target="_blank" rel="noopener noreferrer" sx={{ color: 'inherit' }}>
                    {formatted}{suffix}
                  </Typography>
                ) : (
                  <Typography variant="body2">{formatted}{suffix}</Typography>
                )}
              </Box>
              <Box sx={{ flexShrink: 0, display: 'flex', alignItems: 'center', gap: 0.25 }}>
                {formatted && <CopyButton value={formatted} label={`${t('contactDetail.address')} ${formatted}`} />}
              </Box>
            </Box>
          );
        })}
      </Stack>
    );
  };

  // Shared by SocialProfiles, OtherOnlineServices and IMPP (T44 routes IMPP
  // through this same registry-aware renderer instead of renderUriValueList,
  // so an IMPP handle resolves through the user's LinkFieldType registry the
  // way the other two fields do). `copyLabel` is the field name used in the
  // copy button's aria-label.
  const renderOnlineServices = (rows: CardOnlineService[] | undefined, copyLabel: string) => {
    if (!rows || rows.length === 0) return <Typography variant="body2" color="text.disabled">—</Typography>;
    return (
      <Stack spacing={0.25}>
        {rows.map((s, i) => {
          const label = `${s.service ? `${s.service}: ` : ''}${s.uri || s.user || ''}`;
          const href = resolveOnlineServiceLink(s, linkFieldTypes);
          const suffix = s.contexts?.length ? ` (${s.contexts.join(', ')})` : '';
          const copyValue = s.uri || s.user || label;
          const matched = s.service
            ? linkFieldTypes.find((lt) => lt.name.toLowerCase() === s.service!.toLowerCase())
            : null;
          const iconPath = resolveLinkFieldTypeIcon(matched?.icon);
          return (
            <Box key={i} sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 1 }}>
              <Box sx={{ minWidth: 0, overflowWrap: 'anywhere', display: 'flex', alignItems: 'center', gap: 0.5 }}>
                <SvgIcon fontSize="small" sx={{ flexShrink: 0 }}><path d={iconPath} /></SvgIcon>
                {href ? (
                  <Typography variant="body2" component="a" href={href} target="_blank" rel="noopener noreferrer" sx={{ color: 'inherit', wordBreak: 'break-all' }}>
                    {label}{suffix}
                  </Typography>
                ) : (
                  <Typography variant="body2" sx={{ wordBreak: 'break-all' }}>{label}{suffix}</Typography>
                )}
              </Box>
              <Box sx={{ flexShrink: 0, display: 'flex', alignItems: 'center', gap: 0.25 }}>
                {copyValue && <CopyButton value={copyValue} label={`${copyLabel} ${copyValue}`} />}
              </Box>
            </Box>
          );
        })}
      </Stack>
    );
  };

  const renderPersonalInfo = (rows: CardPersonalInfo[] | undefined) => {
    if (!rows || rows.length === 0) return <Typography variant="body2" color="text.disabled">—</Typography>;
    return (
      <Stack spacing={0.25}>
        {rows.map((p, i) => (
          <Box key={i} sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 1 }}>
            <Box sx={{ minWidth: 0, overflowWrap: 'anywhere' }}>
              <Typography variant="body2">
                {p.value}
                {p.kind ? ` (${t(`contacts.personalInfo.kindOptions.${p.kind}`, p.kind)})` : ''}
                {p.level ? ` · ${t(`contacts.personalInfo.levelOptions.${p.level}`, p.level)}` : ''}
              </Typography>
            </Box>
            <Box sx={{ flexShrink: 0, display: 'flex', alignItems: 'center', gap: 0.25 }}>
              {p.value && <CopyButton value={p.value} label={t('contacts.personalInfoLabel')} />}
            </Box>
          </Box>
        ))}
      </Stack>
    );
  };

  const renderKeywords = (rows: string[] | undefined) => {
    if (!rows || rows.length === 0) return <Typography variant="body2" color="text.disabled">—</Typography>;
    return (
      <Stack direction="row" spacing={0.5} sx={{ flexWrap: 'wrap', alignItems: 'center', gap: 1 }}>
        {rows.map((k, i) => (
          <Box key={i} sx={{ display: 'flex', alignItems: 'center', gap: 0.25 }}>
            <Typography variant="body2">#{k}</Typography>
            <CopyButton value={k} label={t('contacts.keywordsLabel')} />
          </Box>
        ))}
      </Stack>
    );
  };

  const renderCardNotes = (rows: CardNote[] | undefined) => {
    if (!rows || rows.length === 0) return <Typography variant="body2" color="text.disabled">—</Typography>;
    return (
      <Stack spacing={0.25}>
        {rows.map((n, i) => (
          <Box key={i} sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 1 }}>
            <Box sx={{ minWidth: 0, overflowWrap: 'anywhere' }}>
              <Typography variant="body2" sx={{ whiteSpace: 'pre-wrap' }}>{n.note}</Typography>
            </Box>
            <Box sx={{ flexShrink: 0, display: 'flex', alignItems: 'center', gap: 0.25 }}>
              {n.note && <CopyButton value={n.note} label={t('contacts.cardNotesLabel')} />}
            </Box>
          </Box>
        ))}
      </Stack>
    );
  };

  const renderPreferredLanguages = (rows: CardLanguagePref[] | undefined) => {
    if (!rows || rows.length === 0) return <Typography variant="body2" color="text.disabled">—</Typography>;
    return (
      <Stack spacing={0.25}>
        {rows.map((l, i) => (
          <Box key={i} sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 1 }}>
            <Box sx={{ minWidth: 0, overflowWrap: 'anywhere' }}>
              <Typography variant="body2">
                {l.language}
                {l.contexts?.length ? ` (${l.contexts.join(', ')})` : ''}
              </Typography>
            </Box>
            <Box sx={{ flexShrink: 0, display: 'flex', alignItems: 'center', gap: 0.25 }}>
              {l.language && <CopyButton value={l.language} label={t('contacts.preferredLanguagesLabel')} />}
            </Box>
          </Box>
        ))}
      </Stack>
    );
  };

  const renderSpeakToAs = (s: CardSpeakToAs | undefined) => {
    if (!s || (!s.pronouns?.length && !s.grammaticalGenders?.length)) {
      return <Typography variant="body2" color="text.disabled">—</Typography>;
    }
    const parts: string[] = [];
    if (s.pronouns?.length) parts.push(s.pronouns.map((p) => p.pronouns).join(', '));
    if (s.grammaticalGenders?.length) {
      parts.push(s.grammaticalGenders.map((g) => t(`contacts.speakToAs.gramGender.${g.value}`, g.value)).join(', '));
    }
    const combined = parts.join(' · ');
    return (
      <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 1 }}>
        <Box sx={{ minWidth: 0, overflowWrap: 'anywhere' }}>
          <Typography variant="body2">{combined}</Typography>
        </Box>
        <Box sx={{ flexShrink: 0, display: 'flex', alignItems: 'center', gap: 0.25 }}>
          {combined && <CopyButton value={combined} label={t('contacts.speakToAsLabel')} />}
        </Box>
      </Box>
    );
  };

  const renderAnniversaries = (rows: CardAnniversary[] | undefined) => {
    if (!rows || rows.length === 0) return <Typography variant="body2" color="text.disabled">—</Typography>;
    return (
      <Stack spacing={0.25}>
        {rows.map((a, i) => {
          // Raw ISO (YYYY-MM-DD or --MM-DD) is what the copy button shares;
          // the displayed text honors the user's selected date format.
          const iso = formatAnniversaryDate(a.date);
          const date = iso ? formatBirthday(iso) : undefined;
          return (
            <Box key={i} sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 1 }}>
              <Box sx={{ minWidth: 0, overflowWrap: 'anywhere' }}>
                <Typography variant="body2">
                  {date ? date : '—'}
                  {` (${t(`contacts.anniversaryFields.kindOptions.${a.kind}`, a.kind)})`}
                </Typography>
              </Box>
              <Box sx={{ flexShrink: 0, display: 'flex', alignItems: 'center', gap: 0.25 }}>
                {iso && <CopyButton value={iso} label={t('contacts.anniversaries')} />}
              </Box>
            </Box>
          );
        })}
      </Stack>
    );
  };

  return (
    <Card sx={{ minWidth: 0 }}>
      <CardContent sx={{ py: 2 }}>
        {/* T74 Level 1: two-column field-row grid at lg+ so a field's action
            cluster lands ~530px from its value instead of ~1136px. A plain
            Box with gridAutoFlow: 'row' (the default) preserves the current
            top-to-bottom, left-to-right field order -- gap replaces Stack's
            margin-based spacing, so this can't double up with it. Below lg
            it's a single column, byte-identical to today. */}
        <Box
          sx={{
            display: 'grid',
            gridTemplateColumns: { xs: '1fr', lg: 'repeat(2, 1fr)' },
            columnGap: 3,
            rowGap: 2,
            alignItems: 'start',
          }}
        >
          {showAbout && <SectionHeading label={t('contactDetail.section.about')} />}

          {isOn('birthday') && (
            <EditableField
              icon={<CakeIcon sx={iconSx} />}
              label={t('contactDetail.birthday')}
              field="birthday"
              value={birthday}
              formattedDisplayValue={birthday ? formatBirthday(birthday) : undefined}
              placeholder={getBirthdayPlaceholder()}
              displaySuffix={birthdayAgeSuffix}
              isEditing={editingField === 'birthday'}
              editValue={editValue}
              validationError={validationError}
              onEditStart={onEditStart}
              onEditCancel={onEditCancel}
              onEditSave={onEditSave}
              onEditValueChange={onEditValueChange}
            />
          )}

          {isOn('anniversary') && (
            <EditableField
              icon={<CelebrationIcon sx={iconSx} />}
              label={t('contacts.anniversary')}
              field="anniversary"
              value={anniversary}
              formattedDisplayValue={anniversary ? formatBirthday(anniversary) : undefined}
              placeholder={getBirthdayPlaceholder()}
              isEditing={editingField === 'anniversary'}
              editValue={editValue}
              validationError={validationError}
              onEditStart={onEditStart}
              onEditCancel={onEditCancel}
              onEditSave={onEditSave}
              onEditValueChange={onEditValueChange}
            />
          )}

          {isOn('anniversaries') && (
            <EditableArrayField<CardAnniversary[]>
              icon={<CelebrationIcon sx={iconSx} />}
              label={t('contacts.anniversaries')}
              value={card.anniversaries || []}
              cloneValue={(v) => v.map((x) => ({ ...x, date: { ...x.date, partial: x.date.partial ? { ...x.date.partial } : undefined } }))}
              renderDisplay={renderAnniversaries}
              renderEditor={(draft, setDraft) => (
                <AnniversariesEditor label={t('contacts.anniversaries')} value={draft} onChange={setDraft} />
              )}
              onSave={(draft) => onUpdateCard({ anniversaries: draft.length ? draft : undefined })}
            />
          )}

          {isOn('personalInfo') && (
            <EditableArrayField<CardPersonalInfo[]>
              icon={<WorkIcon sx={iconSx} />}
              label={t('contacts.personalInfoLabel')}
              value={card.personalInfo || []}
              cloneValue={(v) => v.map((x) => ({ ...x }))}
              renderDisplay={renderPersonalInfo}
              renderEditor={(draft, setDraft) => (
                <PersonalInfoEditor label={t('contacts.personalInfoLabel')} value={draft} onChange={setDraft} />
              )}
              onSave={(draft) => onUpdateCard({ personalInfo: draft.length ? draft : undefined })}
            />
          )}

          {isOn('preferredLanguages') && (
            <EditableArrayField<CardLanguagePref[]>
              icon={<LanguageIcon sx={iconSx} />}
              label={t('contacts.preferredLanguagesLabel')}
              value={card.preferredLanguages || []}
              cloneValue={(v) => v.map((x) => ({ ...x }))}
              renderDisplay={renderPreferredLanguages}
              renderEditor={(draft, setDraft) => (
                <PreferredLanguagesEditor label={t('contacts.preferredLanguagesLabel')} value={draft} onChange={setDraft} />
              )}
              onSave={(draft) => onUpdateCard({ preferredLanguages: draft.length ? draft : undefined })}
            />
          )}

          {isOn('keywords') && (
            <EditableArrayField<string[]>
              icon={<BadgeIcon sx={iconSx} />}
              label={t('contacts.keywordsLabel')}
              value={card.keywords || []}
              cloneValue={(v) => [...v]}
              renderDisplay={renderKeywords}
              renderEditor={(draft, setDraft) => (
                <KeywordsEditor label={t('contacts.keywordsLabel')} value={draft} onChange={setDraft} />
              )}
              onSave={(draft) => onUpdateCard({ keywords: draft.length ? draft : undefined })}
            />
          )}

          {showContact && <SectionHeading label={t('contactDetail.section.contact')} />}

          {isOn('phones') && (
            <EditableArrayField<ContactValue[]>
              icon={<PhoneIcon sx={iconSx} />}
              label={t('contactDetail.phone')}
              value={cardPhonesToValues(card.phones)}
              cloneValue={cloneValues}
              renderDisplay={renderPhoneList}
              renderEditor={(draft, setDraft) => (
                <MultiValueField label={t('contacts.phone')} value={draft} onChange={setDraft} valueType="tel" defaultType="cell" />
              )}
              onSave={(draft) => onUpdateCard({ phones: valuesToCardPhones(draft) })}
            />
          )}

          {isOn('addresses') && (
            <EditableArrayField<ContactAddress[]>
              icon={<HomeIcon sx={iconSx} />}
              label={t('contactDetail.address')}
              value={cardAddressesToValues(card.addresses)}
              cloneValue={cloneValues}
              renderDisplay={renderAddressList}
              renderEditor={(draft, setDraft) => (
                <AddressFields label={t('contacts.address')} value={draft} onChange={setDraft} />
              )}
              onSave={(draft) => onUpdateCard({ addresses: valuesToCardAddresses(draft) })}
            />
          )}

          {isOn('emails') && (
            <EditableArrayField<ContactValue[]>
              icon={<EmailIcon sx={iconSx} />}
              label={t('contactDetail.email')}
              value={cardEmailsToValues(card.emails)}
              cloneValue={cloneValues}
              renderDisplay={renderEmailList}
              renderEditor={(draft, setDraft) => (
                <MultiValueField label={t('contacts.email')} value={draft} onChange={setDraft} valueType="email" defaultType="home" />
              )}
              onSave={(draft) => onUpdateCard({ emails: valuesToCardEmails(draft) })}
            />
          )}

          {isOn('socialProfiles') && (
            <EditableArrayField<CardOnlineService[]>
              icon={<LanguageIcon sx={iconSx} />}
              label={t('contacts.socialProfiles')}
              value={card.socialProfiles || []}
              cloneValue={(v) => v.map((x) => ({ ...x }))}
              renderDisplay={(rows) => renderOnlineServices(rows, t('contacts.socialProfiles'))}
              renderEditor={(draft, setDraft) => (
                <OnlineServiceEditor
                  label={t('contacts.socialProfiles')}
                  value={draft}
                  onChange={setDraft}
                  showService
                  linkFieldTypes={linkFieldTypes.filter((lt) => lt.category === 'social')}
                />
              )}
              onSave={(draft) => onUpdateCard({ socialProfiles: draft.length ? draft : undefined })}
            />
          )}

          {isOn('otherOnlineServices') && (
            <EditableArrayField<CardOnlineService[]>
              icon={<LanguageIcon sx={iconSx} />}
              label={t('contacts.otherOnlineServices')}
              value={card.otherOnlineServices || []}
              cloneValue={(v) => v.map((x) => ({ ...x }))}
              renderDisplay={(rows) => renderOnlineServices(rows, t('contacts.otherOnlineServices'))}
              renderEditor={(draft, setDraft) => (
                <OnlineServiceEditor
                  label={t('contacts.otherOnlineServices')}
                  value={draft}
                  onChange={setDraft}
                  showService
                  linkFieldTypes={linkFieldTypes.filter((lt) => lt.category === 'other')}
                />
              )}
              onSave={(draft) => onUpdateCard({ otherOnlineServices: draft.length ? draft : undefined })}
            />
          )}

          {isOn('imppAddresses') && (
            <EditableArrayField<CardOnlineService[]>
              icon={<ChatIcon sx={iconSx} />}
              label={t('contacts.impps')}
              value={card.imppAddresses || []}
              cloneValue={cloneValues}
              renderDisplay={(rows) => renderOnlineServices(rows, t('contacts.impps'))}
              renderEditor={(draft, setDraft) => (
                <OnlineServiceEditor
                  label={t('contacts.impps')}
                  value={draft}
                  onChange={setDraft}
                  uriOnly
                  linkFieldTypes={linkFieldTypes.filter((lt) => lt.category === 'messaging')}
                />
              )}
              onSave={(draft) => onUpdateCard({ imppAddresses: draft.length ? draft : undefined })}
            />
          )}

          {isOn('links') && (
            <EditableArrayField<ContactValue[]>
              icon={<LanguageIcon sx={iconSx} />}
              label={t('contacts.urls')}
              value={cardLinksToValues(card.links)}
              cloneValue={cloneValues}
              renderDisplay={(rows) => renderUriValueList(rows, t('contacts.urls'))}
              renderEditor={(draft, setDraft) => (
                <MultiValueField label={t('contacts.urls')} value={draft} onChange={setDraft} valueType="url" defaultType="home" />
              )}
              onSave={(draft) => onUpdateCard({ links: valuesToCardLinks(draft) })}
            />
          )}

          {showAddressThem && <SectionHeading label={t('contactDetail.section.genderAndPronouns')} />}

          {isOn('gender') && (
            <EditableField
              icon={<PersonOutlineIcon sx={iconSx} />}
              label={t('contacts.gender')}
              field="gender"
              value={gender}
              formattedDisplayValue={genderDisplay}
              isEditing={editingField === 'gender'}
              editValue={editValue}
              validationError={validationError}
              onEditStart={onEditStart}
              onEditCancel={onEditCancel}
              onEditSave={onEditSave}
              onEditValueChange={onEditValueChange}
              options={[...GENDER_OPTIONS]}
              getOptionLabel={(v) => (GENDER_OPTIONS.includes(v as (typeof GENDER_OPTIONS)[number]) ? t(`contacts.${v}`) : v)}
              placeholder={t('contacts.selectGender')}
            />
          )}

          {isOn('speakToAs') && (
            <FullSpanField>
              <EditableArrayField<CardSpeakToAs>
                icon={<BadgeIcon sx={iconSx} />}
                label={t('contacts.speakToAsLabel')}
                value={card.speakToAs || { grammaticalGenders: [], pronouns: [] }}
                cloneValue={(v) => ({
                  grammaticalGenders: (v.grammaticalGenders || []).map((g) => ({ ...g })),
                  pronouns: (v.pronouns || []).map((p) => ({ ...p })),
                })}
                renderDisplay={renderSpeakToAs}
                renderEditor={(draft, setDraft) => (
                  <SpeakToAsEditor value={draft} onChange={setDraft} />
                )}
                onSave={(draft) => onUpdateCard({
                  speakToAs: draft.pronouns?.length || draft.grammaticalGenders?.length ? draft : undefined,
                })}
              />
            </FullSpanField>
          )}

          {showProfessional && <SectionHeading label={t('contactDetail.section.professional')} />}

          {isOn('organizations') && (
            <>
              <EditableField
                icon={<BusinessIcon sx={iconSx} />}
                label={t('contacts.organization')}
                field="organization"
                value={organization}
                isEditing={editingField === 'organization'}
                editValue={editValue}
                validationError={validationError}
                onEditStart={onEditStart}
                onEditCancel={onEditCancel}
                onEditSave={onEditSave}
                onEditValueChange={onEditValueChange}
              />
              <EditableField
                icon={<BusinessIcon sx={iconSx} />}
                label={t('contacts.department')}
                field="department"
                value={department}
                isEditing={editingField === 'department'}
                editValue={editValue}
                validationError={validationError}
                onEditStart={onEditStart}
                onEditCancel={onEditCancel}
                onEditSave={onEditSave}
                onEditValueChange={onEditValueChange}
              />
            </>
          )}

          {isOn('titles') && (
            <>
              <EditableField
                icon={<BadgeIcon sx={iconSx} />}
                label={t('contacts.jobTitle')}
                field="job_title"
                value={jobTitle}
                isEditing={editingField === 'job_title'}
                editValue={editValue}
                validationError={validationError}
                onEditStart={onEditStart}
                onEditCancel={onEditCancel}
                onEditSave={onEditSave}
                onEditValueChange={onEditValueChange}
              />
              <EditableField
                icon={<BadgeIcon sx={iconSx} />}
                label={t('contacts.role')}
                field="role"
                value={role}
                isEditing={editingField === 'role'}
                editValue={editValue}
                validationError={validationError}
                onEditStart={onEditStart}
                onEditCancel={onEditCancel}
                onEditSave={onEditSave}
                onEditValueChange={onEditValueChange}
              />
            </>
          )}

          {isOn('work_information') && (
            <FullSpanField>
              <EditableField
                icon={<WorkIcon sx={{ ...iconSx, mt: 0.5 }} />}
                label={t('contactDetail.workInfo')}
                field="work_information"
                value={crm.work_information || ''}
                multiline
                isEditing={editingField === 'work_information'}
                editValue={editValue}
                validationError={validationError}
                onEditStart={onEditStart}
                onEditCancel={onEditCancel}
                onEditSave={onEditSave}
                onEditValueChange={onEditValueChange}
              />
            </FullSpanField>
          )}

          {showNotes && <SectionHeading label={t('contactDetail.section.notes')} />}

          {isOn('cardNotes') && (
            <FullSpanField>
              <EditableArrayField<CardNote[]>
                icon={<SvgIcon sx={iconSx}><path d={mdiNoteMultipleOutline} /></SvgIcon>}
                label={t('contacts.cardNotesLabel')}
                value={card.notes || []}
                cloneValue={(v) => v.map((x) => ({ ...x, author: x.author ? { ...x.author } : undefined }))}
                renderDisplay={renderCardNotes}
                renderEditor={(draft, setDraft) => (
                  <CardNotesEditor label={t('contacts.cardNotesLabel')} value={draft} onChange={setDraft} />
                )}
                onSave={(draft) => onUpdateCard({ notes: draft.length ? draft : undefined })}
              />
            </FullSpanField>
          )}

          {isOn('how_we_met') && (
            <FullSpanField>
              <EditableField
                icon={<PeopleIcon sx={{ ...iconSx, mt: 0.5 }} />}
                label={t('contactDetail.howWeMet')}
                field="how_we_met"
                value={crm.how_we_met || ''}
                multiline
                isEditing={editingField === 'how_we_met'}
                editValue={editValue}
                validationError={validationError}
                onEditStart={onEditStart}
                onEditCancel={onEditCancel}
                onEditSave={onEditSave}
                onEditValueChange={onEditValueChange}
              />
            </FullSpanField>
          )}

          {isOn('contact_information') && (
            <FullSpanField>
              <EditableField
                icon={<SvgIcon sx={{ ...iconSx, mt: 0.5 }}><path d={mdiNoteMultipleOutline} /></SvgIcon>}
                label={t('contactDetail.additionalInfo')}
                field="contact_information"
                value={crm.contact_information || ''}
                multiline
                isEditing={editingField === 'contact_information'}
                editValue={editValue}
                validationError={validationError}
                onEditStart={onEditStart}
                onEditCancel={onEditCancel}
                onEditSave={onEditSave}
                onEditValueChange={onEditValueChange}
              />
            </FullSpanField>
          )}

          {/* Custom Fields (v2, T6/T7): one row per FieldDefinition, rendered
              per type by CustomFieldValueRow. */}
          {fieldDefinitions.map((definition) => (
            <CustomFieldValueRow
              key={definition.id}
              definition={definition}
              value={fieldValuesByDefinition?.get(definition.id)}
              onSave={onSaveFieldValue || (() => Promise.resolve())}
            />
          ))}

          {/* Card metadata: the read-only imported-resource + relatedTo/members
              sections (WP7/WP8) — present to prevent silent data loss and to
              make imported data visible, not editable (full editing UI is
              T29b). Grouped under the same "Card metadata" heading the header
              and Add-dialog use for language/contact-kind. Collapsible because
              these lists can grow arbitrarily large (v0.4.1). */}
          {showMetadata && (
            <Box sx={{ gridColumn: { lg: '1 / -1' } }}>
              <Button
                fullWidth
                onClick={() => setMetadataExpanded((v) => !v)}
                aria-expanded={metadataExpanded}
                aria-label={
                  metadataExpanded
                    ? t('contactDetail.section.metadataCollapse')
                    : t('contactDetail.section.metadataExpand')
                }
                endIcon={
                  <ExpandMoreIcon
                    sx={{ transform: metadataExpanded ? 'rotate(180deg)' : 'none', transition: 'transform 0.2s' }}
                  />
                }
                sx={{
                  justifyContent: 'space-between',
                  px: 0,
                  py: 0.5,
                  color: 'text.secondary',
                  textTransform: 'none',
                  fontWeight: 'inherit',
                  '&:hover': { bgcolor: 'transparent', color: 'text.primary' },
                }}
              >
                <Typography variant="overline" sx={{ letterSpacing: 0.08, fontSize: '0.72rem' }}>
                  {t('contactDetail.section.metadata')}
                </Typography>
              </Button>
              <Divider sx={{ mt: 0.5 }} />
              <Collapse in={metadataExpanded}>
                <ImportedResourcesSection card={card} />
                <RelatedToMembersSection card={card} />
              </Collapse>
            </Box>
          )}
        </Box>
      </CardContent>
    </Card>
  );
}