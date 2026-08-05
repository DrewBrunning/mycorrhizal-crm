import { useState, useMemo, useEffect } from 'react';
import { Card, CardContent, Divider, Stack, Box, Tabs, Tab, Button, Typography, SvgIcon, TextField, MenuItem, IconButton, Tooltip, useTheme, useMediaQuery } from '@mui/material';import EmailIcon from '@mui/icons-material/Email';
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
import AddIcon from '@mui/icons-material/Add';
import { useTranslation } from 'react-i18next';
import EditableField from './EditableField';
import EditableArrayField from './EditableArrayField';
import MultiValueField from './MultiValueField';
import AddressFields from './AddressFields';
import RelationshipEdgeList from './RelationshipEdgeList';
import LifeEventList from './LifeEventList';
import PreferenceList from './PreferenceList';
import CadencePanel from './CadencePanel';
import ConversationAgendaList from './ConversationAgendaList';
import GiftList from './GiftList';
import ExternalLinkPanel from './ExternalLinkPanel';
import ConnectionsPanel from './ConnectionsPanel';
import CustomFieldValueRow from './CustomFieldValueRow';
import { RelationshipEdge } from '../api/relationshipEdges';
import { LifeEvent } from '../api/lifeEvents';
import { Preference } from '../api/preferences';
import { CadencePolicy } from '../api/cadencePolicies';
import { ConversationAgenda } from '../api/conversationAgenda';
import { Gift } from '../api/gifts';
import { Activity } from '../api/activities';
import { FieldDefinition } from '../api/fieldDefinitions';
import { ExternalIdentity } from '../api/externalLinks';
import { ImmichPerson, ImmichPersonSummary } from '../api/immich';
import {
  Card as CardModel,
  CRMEnvelope,
  Contact,
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
  cardImppToValues,
  valuesToCardImpp,
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
import ClothingSizesPanel from './ClothingSizesPanel';
import { PREFERENCE_CLOTHING_SIZE } from '../api/preferences';
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
  // RelationshipEdge props (§3d WP3)
  confirmedEdges?: RelationshipEdge[];
  suggestedEdges?: RelationshipEdge[];
  contactsByUid?: Map<string, Contact>;
  viewedContactUid?: string;
  onAddRelationshipEdge?: () => void;
  onEditRelationshipEdge?: (edge: RelationshipEdge) => void;
  onDeleteRelationshipEdge?: (edgeId: string) => void;
  onAcceptSuggestion?: (edgeId: string) => void;
  onRejectSuggestion?: (edgeId: string) => void;
  // LifeEvent props (T5)
  lifeEvents?: LifeEvent[];
  lifeEventsContactsByUid?: Map<string, Contact>;
  onAddLifeEvent?: () => void;
  onEditLifeEvent?: (event: LifeEvent) => void;
  onDeleteLifeEvent?: (id: string) => void;
  // Preference props (T20a)
  preferences?: Preference[];
  onAddPreference?: () => void;
  onEditPreference?: (preference: Preference) => void;
  onDeletePreference?: (id: string) => void;
  // Cadence props (T19)
  cadencePolicy?: CadencePolicy | null;
  cadenceLoading?: boolean;
  onAddCadence?: () => void;
  onEditCadence?: (policy: CadencePolicy) => void;
  onDeleteCadence?: (id: string) => void;
  // Conversation agenda props (T21)
  agendaItems?: ConversationAgenda[];
  onAddAgenda?: (content: string) => Promise<void>;
  onEditAgenda?: (item: ConversationAgenda) => void;
  onDiscussAgenda?: (item: ConversationAgenda) => void;
  onDeleteAgenda?: (id: string) => void;
  // Gift props (T20b)
  gifts?: Gift[];
  activities?: Activity[];
  onAddGift?: (description: string) => Promise<void>;
  onEditGift?: (gift: Gift) => void;
  onMarkGivenGift?: (gift: Gift) => Promise<void>;
  onDeleteGift?: (id: string) => void;
  // Clothing sizes (stored as clothing_size preferences, surfaced in the Gifts
  // tab where you check sizes before buying).
  onAddClothingSize?: (value: string) => Promise<void>;
  onEditClothingSize?: (preference: Preference, value: string) => Promise<void>;
  onDeleteClothingSize?: (id: string) => Promise<void>;
  // Custom fields (v2, T6/T7)
  fieldDefinitions?: FieldDefinition[];
  fieldValuesByDefinition?: Map<string, unknown>;
  onSaveFieldValue?: (definitionId: string, value: unknown) => Promise<void>;
  // External links (T14) + Immich surface (T15/T16)
  externalIdentities?: ExternalIdentity[];
  externalLinksLoading?: boolean;
  immichSummary?: ImmichPersonSummary | null;
  immichSummaryLoading?: boolean;
  onFetchImmichPeople?: () => Promise<ImmichPerson[]>;
  onLinkImmich?: (person: ImmichPerson) => Promise<void>;
  onUnlinkImmich?: () => Promise<void>;
  onSyncImmich?: () => Promise<void>;
  immichSyncing?: boolean;
  // Graph traversal (T10) — the anchor is the viewed contact.
  connectionsEnabled?: boolean;
}

const iconSx = { mr: 1, color: 'text.secondary', fontSize: '1.2rem' };
const cloneValues = <T extends object>(v: T[]): T[] => v.map((x) => ({ ...x }));

// Small section headings for the General Info tab, matching the caption-style
// group labels the header uses for circles/tags.
const SectionHeading = ({ label }: { label: string }) => (
  <Box sx={{ pt: 1 }}>
    <Typography variant="overline" color="text.secondary" sx={{ letterSpacing: 0.08, fontSize: '0.72rem' }}>
      {label}
    </Typography>
    <Divider sx={{ mt: 0.5 }} />
  </Box>
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
  confirmedEdges = [],
  suggestedEdges = [],
  contactsByUid,
  viewedContactUid,
  onAddRelationshipEdge,
  onEditRelationshipEdge,
  onDeleteRelationshipEdge,
  onAcceptSuggestion,
  onRejectSuggestion,
  lifeEvents = [],
  lifeEventsContactsByUid,
  onAddLifeEvent,
  onEditLifeEvent,
  onDeleteLifeEvent,
  preferences = [],
  onAddPreference,
  onEditPreference,
  onDeletePreference,
  cadencePolicy = null,
  cadenceLoading = false,
  onAddCadence,
  onEditCadence,
  onDeleteCadence,
  agendaItems = [],
  onAddAgenda,
  onEditAgenda,
  onDiscussAgenda,
  onDeleteAgenda,
  gifts = [],
  activities = [],
  onAddGift,
  onEditGift,
  onMarkGivenGift,
  onDeleteGift,
  onAddClothingSize,
  onEditClothingSize,
  onDeleteClothingSize,
  fieldDefinitions = [],
  fieldValuesByDefinition,
  onSaveFieldValue,
  externalIdentities = [],
  externalLinksLoading = false,
  immichSummary = null,
  immichSummaryLoading = false,
  onFetchImmichPeople,
  onLinkImmich,
  onUnlinkImmich,
  onSyncImmich,
  immichSyncing = false,
  connectionsEnabled = true,
}: ContactInformationProps) {
  const { t } = useTranslation();
  const { formatBirthday, getBirthdayPlaceholder, calculateAge } = useDateFormat();
  const [activeTab, setActiveTab] = useState(0);
  const enabled = enabledFields ?? resolveEnabledFields(null);
  const isOn = (key: ContactFieldKey) => enabled.has(key);
  // Read-only fetch of the user's LinkFieldType registry (T34) — resolves
  // socialProfiles/otherOnlineServices handles to tappable links. Only
  // fetched when one of those fields is actually enabled: ListLinkFieldTypes
  // does a write (lazy-seeds 19 defaults) on a user's first call, and
  // neither field is in DEFAULT_ENABLED_CONTACT_FIELDS, so an unconditional
  // fetch would cost every contact-page view for accounts that never touch
  // this registry.
  const socialFieldsEnabled = isOn('socialProfiles') || isOn('otherOnlineServices');
  const { linkFieldTypes, refreshLinkFieldTypes } = useLinkFieldTypes();
  useEffect(() => {
    if (socialFieldsEnabled) refreshLinkFieldTypes();
  }, [socialFieldsEnabled, refreshLinkFieldTypes]);
  // On viewports ≤600px, swap the horizontal tab bar for a dropdown Select
  // that avoids both overflow cut-off and the need for drag-gesture-less
  // scrollable tabs (T28).
  const theme = useTheme();
  const compactTabs = useMediaQuery(theme.breakpoints.down('sm'));

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
            <Box key={i} sx={{ display: 'flex', alignItems: 'center', gap: 0.25 }}>
              <Typography variant="body2">
                {r.value}
                {r.type ? ` (${t(`contacts.types.${r.type}`, r.type)})` : ''}
              </Typography>
              {!isFax && (
                <Tooltip title={t('contactDetail.call')}>
                  <IconButton size="small" component="a" href={buildTelLink(r.value)} aria-label={`${t('contactDetail.call')} ${r.value}`}>
                    <PhoneIcon fontSize="inherit" />
                  </IconButton>
                </Tooltip>
              )}
              {isCell && (
                <Tooltip title={t('contactDetail.text')}>
                  <IconButton size="small" component="a" href={buildSmsLink(r.value)} aria-label={`${t('contactDetail.text')} ${r.value}`}>
                    <SmsOutlinedIcon fontSize="inherit" />
                  </IconButton>
                </Tooltip>
              )}
              <CopyButton value={r.value} label={`${t('contactDetail.phone')} ${r.value}`} />
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
          <Box key={i} sx={{ display: 'flex', alignItems: 'center', gap: 0.25 }}>
            <Typography variant="body2" component="a" href={buildMailtoLink(r.value)} sx={{ color: 'inherit' }}>
              {r.value}
              {r.type ? ` (${t(`contacts.types.${r.type}`, r.type)})` : ''}
            </Typography>
            <CopyButton value={r.value} label={`${t('contactDetail.email')} ${r.value}`} />
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
            <Box key={i} sx={{ display: 'flex', alignItems: 'center', gap: 0.25 }}>
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
              {r.value && <CopyButton value={r.value} label={`${copyLabel} ${r.value}`} />}
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
            <Box key={i} sx={{ display: 'flex', alignItems: 'center', gap: 0.25 }}>
              {href ? (
                <Typography variant="body2" component="a" href={href} target="_blank" rel="noopener noreferrer" sx={{ color: 'inherit' }}>
                  {formatted}{suffix}
                </Typography>
              ) : (
                <Typography variant="body2">{formatted}{suffix}</Typography>
              )}
              {formatted && <CopyButton value={formatted} label={`${t('contactDetail.address')} ${formatted}`} />}
            </Box>
          );
        })}
      </Stack>
    );
  };

  const renderOnlineServices = (rows: CardOnlineService[] | undefined) => {
    if (!rows || rows.length === 0) return <Typography variant="body2" color="text.disabled">—</Typography>;
    return (
      <Stack spacing={0.25}>
        {rows.map((s, i) => {
          const label = `${s.service ? `${s.service}: ` : ''}${s.uri || s.user || ''}`;
          const href = resolveOnlineServiceLink(s, linkFieldTypes);
          const suffix = s.contexts?.length ? ` (${s.contexts.join(', ')})` : '';
          const copyValue = s.uri || s.user || label;
          return (
            <Box key={i} sx={{ display: 'flex', alignItems: 'center', gap: 0.25 }}>
              {href ? (
                <Typography variant="body2" component="a" href={href} target="_blank" rel="noopener noreferrer" sx={{ color: 'inherit', wordBreak: 'break-all' }}>
                  {label}{suffix}
                </Typography>
              ) : (
                <Typography variant="body2" sx={{ wordBreak: 'break-all' }}>{label}{suffix}</Typography>
              )}
              {copyValue && <CopyButton value={copyValue} label={`${t('contacts.socialProfiles')} ${copyValue}`} />}
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
          <Box key={i} sx={{ display: 'flex', alignItems: 'center', gap: 0.25 }}>
            <Typography variant="body2">
              {p.value}
              {p.kind ? ` (${t(`contacts.personalInfo.kindOptions.${p.kind}`, p.kind)})` : ''}
              {p.level ? ` · ${t(`contacts.personalInfo.levelOptions.${p.level}`, p.level)}` : ''}
            </Typography>
            {p.value && <CopyButton value={p.value} label={t('contacts.personalInfoLabel')} />}
          </Box>
        ))}
      </Stack>
    );
  };

  const renderKeywords = (rows: string[] | undefined) => {
    if (!rows || rows.length === 0) return <Typography variant="body2" color="text.disabled">—</Typography>;
    return (
      <Stack direction="row" spacing={0.5} sx={{ flexWrap: 'wrap', alignItems: 'center' }}>
        {rows.map((k, i) => (
          <Box key={i} sx={{ display: 'flex', alignItems: 'center' }}>
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
          <Box key={i} sx={{ display: 'flex', alignItems: 'center', gap: 0.25 }}>
            <Typography variant="body2" sx={{ whiteSpace: 'pre-wrap' }}>{n.note}</Typography>
            {n.note && <CopyButton value={n.note} label={t('contacts.cardNotesLabel')} />}
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
          <Box key={i} sx={{ display: 'flex', alignItems: 'center', gap: 0.25 }}>
            <Typography variant="body2">
              {l.language}
              {l.contexts?.length ? ` (${l.contexts.join(', ')})` : ''}
            </Typography>
            {l.language && <CopyButton value={l.language} label={t('contacts.preferredLanguagesLabel')} />}
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
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.25 }}>
        <Typography variant="body2">{combined}</Typography>
        {combined && <CopyButton value={combined} label={t('contacts.speakToAsLabel')} />}
      </Box>
    );
  };

  const renderAnniversaries = (rows: CardAnniversary[] | undefined) => {
    if (!rows || rows.length === 0) return <Typography variant="body2" color="text.disabled">—</Typography>;
    return (
      <Stack spacing={0.25}>
        {rows.map((a, i) => {
          const date = formatAnniversaryDate(a.date);
          return (
            <Box key={i} sx={{ display: 'flex', alignItems: 'center', gap: 0.25 }}>
              <Typography variant="body2">
                {date ? date : '—'}
                {` (${t(`contacts.anniversaryFields.kindOptions.${a.kind}`, a.kind)})`}
              </Typography>
              {date && <CopyButton value={date} label={t('contacts.anniversaries')} />}
            </Box>
          );
        })}
      </Stack>
    );
  };

  return (
    <Card sx={{ flex: 1, minWidth: 0 }}>
      {compactTabs ? (
        <TextField
          select
          size="small"
          value={activeTab}
          onChange={(e) => setActiveTab(Number(e.target.value))}
          sx={{ m: 1, minWidth: 180 }}
          aria-label="contact information sections"
        >
          <MenuItem value={0}>{t('contactDetail.generalInfo')}</MenuItem>
          <MenuItem value={1}>{t('relationships.title')}</MenuItem>
          <MenuItem value={2}>{t('lifeEvent.title')}</MenuItem>
          <MenuItem value={3}>{t('preference.title')}</MenuItem>
          <MenuItem value={4}>{t('cadence.title')}</MenuItem>
          <MenuItem value={5}>{t('conversationAgenda.title')}</MenuItem>
          <MenuItem value={6}>{t('gifts.title')}</MenuItem>
          <MenuItem value={7}>{t('externalLinks.title')}</MenuItem>
          <MenuItem value={8}>{t('connections.title')}</MenuItem>
        </TextField>
      ) : (
        <Box sx={{ borderBottom: 1, borderColor: 'divider' }}>
          <Tabs value={activeTab} onChange={(_, newValue) => setActiveTab(newValue)} aria-label="contact information tabs">
            <Tab label={t('contactDetail.generalInfo')} />
            <Tab label={t('relationships.title')} />
            <Tab label={t('lifeEvent.title')} />
            <Tab label={t('preference.title')} />
            <Tab label={t('cadence.title')} />
            <Tab label={t('conversationAgenda.title')} />
            <Tab label={t('gifts.title')} />
            <Tab label={t('externalLinks.title')} />
            <Tab label={t('connections.title')} />
          </Tabs>
        </Box>
      )}

      {/* General Information Tab */}
      {activeTab === 0 && (
        <CardContent sx={{ py: 2 }}>
          <Stack spacing={2}>
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
                renderDisplay={renderOnlineServices}
                renderEditor={(draft, setDraft) => (
                  <OnlineServiceEditor
                    label={t('contacts.socialProfiles')}
                    value={draft}
                    onChange={setDraft}
                    showService
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
                renderDisplay={renderOnlineServices}
                renderEditor={(draft, setDraft) => (
                  <OnlineServiceEditor
                    label={t('contacts.otherOnlineServices')}
                    value={draft}
                    onChange={setDraft}
                    showService
                  />
                )}
                onSave={(draft) => onUpdateCard({ otherOnlineServices: draft.length ? draft : undefined })}
              />
            )}

            {isOn('imppAddresses') && (
              <EditableArrayField<ContactValue[]>
                icon={<ChatIcon sx={iconSx} />}
                label={t('contacts.impps')}
                value={cardImppToValues(card.imppAddresses)}
                cloneValue={cloneValues}
                renderDisplay={(rows) => renderUriValueList(rows, t('contacts.impps'))}
                renderEditor={(draft, setDraft) => (
                  <MultiValueField label={t('contacts.impps')} value={draft} onChange={setDraft} defaultType="" freeTextType />
                )}
                onSave={(draft) => onUpdateCard({ imppAddresses: valuesToCardImpp(draft) })}
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
              />
            )}

            {isOn('speakToAs') && (
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
            )}

            {showNotes && <SectionHeading label={t('contactDetail.section.notes')} />}

            {isOn('cardNotes') && (
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
            )}

            {isOn('how_we_met') && (
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
            )}

            {isOn('contact_information') && (
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
                and Add-dialog use for language/contact-kind. */}
            {showMetadata && <SectionHeading label={t('contactDetail.section.metadata')} />}
            <ImportedResourcesSection card={card} />
            <RelatedToMembersSection card={card} />
          </Stack>
        </CardContent>
      )}

      {/* Relationships Tab */}
      {activeTab === 1 && (
        <CardContent sx={{ py: 2 }}>
          <Box sx={{ display: 'flex', justifyContent: 'flex-end', mb: 1.5 }}>
            <Button
              startIcon={<AddIcon />}
              onClick={onAddRelationshipEdge}
              variant="outlined"
              size="small"
            >
              {t('relationships.addRelationship')}
            </Button>
          </Box>
          <Divider sx={{ mb: 1.5 }} />
          <RelationshipEdgeList
            confirmedEdges={confirmedEdges}
            suggestedEdges={suggestedEdges}
            contactsByUid={contactsByUid || new Map()}
            viewedContactUid={viewedContactUid || ''}
            onEdit={onEditRelationshipEdge || (() => {})}
            onDelete={onDeleteRelationshipEdge || (() => {})}
            onAccept={onAcceptSuggestion || (() => {})}
            onReject={onRejectSuggestion || (() => {})}
          />
        </CardContent>
      )}

      {/* Life Events Tab */}
      {activeTab === 2 && (
        <CardContent sx={{ py: 2 }}>
          <Box sx={{ display: 'flex', justifyContent: 'flex-end', mb: 1.5 }}>
            <Button
              startIcon={<AddIcon />}
              onClick={onAddLifeEvent}
              variant="outlined"
              size="small"
            >
              {t('lifeEvent.add')}
            </Button>
          </Box>
          <Divider sx={{ mb: 1.5 }} />
          <LifeEventList
            events={lifeEvents}
            contactsByUid={lifeEventsContactsByUid || new Map()}
            onEdit={onEditLifeEvent || (() => {})}
            onDelete={onDeleteLifeEvent || (() => {})}
          />
        </CardContent>
      )}

      {/* Preferences Tab (T20a) */}
      {activeTab === 3 && (
        <CardContent sx={{ py: 2 }}>
          <Box sx={{ display: 'flex', justifyContent: 'flex-end', mb: 1.5 }}>
            <Button
              startIcon={<AddIcon />}
              onClick={onAddPreference}
              variant="outlined"
              size="small"
            >
              {t('preference.add')}
            </Button>
          </Box>
          <Divider sx={{ mb: 1.5 }} />
          <PreferenceList
            preferences={preferences}
            onEdit={onEditPreference || (() => {})}
            onDelete={onDeletePreference || (() => {})}
          />
        </CardContent>
      )}

      {/* Cadence Tab (T19) */}
      {activeTab === 4 && (
        <CardContent sx={{ py: 2 }}>
          <CadencePanel
            policy={cadencePolicy}
            loading={cadenceLoading}
            onAdd={onAddCadence || (() => {})}
            onEdit={onEditCadence || (() => {})}
            onDelete={onDeleteCadence || (() => {})}
          />
        </CardContent>
      )}

      {/* Conversation Agenda Tab (T21) — always-visible list with a single
          inline input; no modal for adding, per the ticket's low-friction
          requirement. */}
      {activeTab === 5 && (
        <CardContent sx={{ py: 2 }}>
          <ConversationAgendaList
            items={agendaItems}
            onAdd={onAddAgenda || (() => Promise.resolve())}
            onEdit={onEditAgenda || (() => {})}
            onDiscuss={onDiscussAgenda || (() => {})}
            onDelete={onDeleteAgenda || (() => {})}
          />
        </CardContent>
      )}

      {/* Gifts Tab (T20b) — inline idea capture at the top, then open ideas
          and the resolved given/received records. Clothing sizes live here too:
          they matter when you are about to buy. */}
      {activeTab === 6 && (
        <CardContent sx={{ py: 2 }}>
          <ClothingSizesPanel
            sizes={preferences.filter((p) => p.category === PREFERENCE_CLOTHING_SIZE)}
            onAdd={onAddClothingSize || (() => Promise.resolve())}
            onEdit={onEditClothingSize || (() => Promise.resolve())}
            onDelete={onDeleteClothingSize || (() => Promise.resolve())}
          />
          {preferences.some((p) => p.category === PREFERENCE_CLOTHING_SIZE) && <Divider sx={{ my: 1.5 }} />}
          <GiftList
            items={gifts}
            lifeEvents={lifeEvents}
            activities={activities}
            onAdd={onAddGift || (() => Promise.resolve())}
            onEdit={onEditGift || (() => {})}
            onMarkGiven={onMarkGivenGift || (() => Promise.resolve())}
            onDelete={onDeleteGift || (() => {})}
          />
        </CardContent>
      )}

      {/* External links Tab (T14 + T15/T16) — the generic integration
          substrate's surface, with the Immich link flow rendered on top. */}
      {activeTab === 7 && (
        <CardContent sx={{ py: 2 }}>
          <ExternalLinkPanel
            contactUid={viewedContactUid || ''}
            identities={externalIdentities}
            loading={externalLinksLoading}
            immichSummary={immichSummary}
            immichSummaryLoading={immichSummaryLoading}
            onFetchImmichPeople={onFetchImmichPeople || (() => Promise.resolve([]))}
            onLinkImmich={onLinkImmich || (() => Promise.resolve())}
            onUnlinkImmich={onUnlinkImmich || (() => Promise.resolve())}
            onSyncImmich={onSyncImmich || (() => Promise.resolve())}
            syncing={immichSyncing}
          />
        </CardContent>
      )}

      {/* Connections Tab (T10) — multi-hop traversal from the viewed contact. */}
      {activeTab === 8 && (
        <CardContent sx={{ py: 2 }}>
          {connectionsEnabled !== false && viewedContactUid ? (
            <ConnectionsPanel contactUid={viewedContactUid} />
          ) : (
            <Typography variant="body2" color="text.secondary">
              {t('connections.notAvailable')}
            </Typography>
          )}
        </CardContent>
      )}
    </Card>
  );
}
