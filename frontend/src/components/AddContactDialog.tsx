import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  TextField,
  Autocomplete,
  MenuItem,
  Chip,
  Box,
  Typography,
  Stack,
  FormControlLabel,
  Switch,
  Divider,
} from '@mui/material';
import AppDialog from './AppDialog';
import MultiValueField from './MultiValueField';
import AddressFields from './AddressFields';
import FieldValueEditor from './FieldValueEditor';
import SpeakToAsEditor from './SpeakToAsEditor';
import PersonalInfoEditor from './PersonalInfoEditor';
import LanguageField from './LanguageField';
import {
  createContactRecord,
  ContactValue,
  ContactAddress,
  NameComponent,
  CardSpeakToAs,
  CardPersonalInfo,
  valuesToCardEmails,
  valuesToCardPhones,
  valuesToCardLinks,
  valuesToCardImpp,
  valuesToCardAddresses,
  withAnniversary,
  withOrganization,
  withTitles,
} from '../api/contacts';
import {
  FieldDefinition,
  FieldValueEditorState,
  FieldValueInput,
  emptyEditorValue,
  editorToWireValue,
  isEditorValueEmpty,
} from '../api/fieldDefinitions';
import { replaceContactFieldValues } from '../api/fieldDefinitions';
import { Circle } from '../api/circles';
import { addCircleMember, createCircle } from '../api/circles';
import { Tag } from '../api/tags';
import { createTag, addContactTag } from '../api/tags';
import { createReminder } from '../api/reminders';
import { useSnackbar } from '../context/SnackbarContext';
import { handleError, getErrorMessage } from '../utils/errorHandler';
import { useDateFormat } from '../DateFormatProvider';
import { ContactFieldKey, resolveEnabledFields, GENDER_OPTIONS } from '../contactFields';
import i18n from '../i18n/config';

interface AddContactDialogProps {
  open: boolean;
  onClose: () => void;
  onContactAdded: (contactId: number) => void;
  availableCircles: Circle[];
  availableTags: Tag[];
  fieldDefinitions?: FieldDefinition[];
  enabledFields?: Set<ContactFieldKey>;
}

const emptyForm = {
  firstname: '',
  lastname: '',
  prefix: '',
  middle_name: '',
  suffix: '',
  nickname: '',
  gender: '',
  // CRMEnvelope.Kind (T27): human|animal. Defaults to human —
  // the suggestion engine treats it the same as an unset kind (classAdult).
  kind: 'human',
  // Card.Kind (WP13, T29): individual|group|org|location|application|device.
  cardKind: '',
  // Card.Language (WP4, T29): the vCard's language tag. Defaults to the user's
  // display language — this is card metadata, not the language the contact
  // speaks (see LanguageField's info tooltip).
  language: '',
  birthday: '',
  anniversary: '',
  organization: '',
  department: '',
  job_title: '',
  role: '',
  how_we_met: '',
  work_information: '',
  contact_information: ''
};

export default function AddContactDialog({
  open,
  onClose,
  onContactAdded,
  availableCircles,
  availableTags,
  fieldDefinitions = [],
  enabledFields
}: AddContactDialogProps) {
  const { t } = useTranslation();
  const { showError, showSuccess } = useSnackbar();
  const { parseBirthdayInput, getBirthdayPlaceholder, autoFormatBirthdayInput } = useDateFormat();
  const enabled = enabledFields ?? resolveEnabledFields(null);
  const isOn = (key: ContactFieldKey) => enabled.has(key);
  // A form section's heading is shown only when at least one of its fields is
  // enabled. The create form is always empty, so "would this section render
  // anything" reduces to the field-visibility check alone (T30) — mirroring
  // the detail page's hasVisibleFields but without the "has a value" half.
  const sectionEnabled = (keys: ContactFieldKey[]) => keys.some(isOn);

  // The vCard's default language tag is prefilled from the language the user is
  // displaying the system in (e.g. "de" from "de-AT"). Region subtags are
  // dropped — the base ISO-639 code is what the dropdown enumerates.
  const defaultLanguage = () => (i18n.language || 'en').split('-')[0];

  // Section headings for the Add Contact form, matching the overline style used
  // by the General Info tab on the contact detail page.
  const SectionHeading = ({ label }: { label: string }) => (
    <Box>
      <Typography variant="overline" color="text.secondary" sx={{ letterSpacing: 0.08, fontSize: '0.72rem' }}>
        {label}
      </Typography>
      <Divider sx={{ mt: 0.5 }} />
    </Box>
  );

  const [formData, setFormData] = useState(() => ({ ...emptyForm, language: defaultLanguage() }));
  const [emails, setEmails] = useState<ContactValue[]>([]);
  const [phones, setPhones] = useState<ContactValue[]>([]);
  const [addresses, setAddresses] = useState<ContactAddress[]>([]);
  const [urls, setUrls] = useState<ContactValue[]>([]);
  const [impps, setImpps] = useState<ContactValue[]>([]);
  const [speakToAs, setSpeakToAs] = useState<CardSpeakToAs>({ grammaticalGenders: [], pronouns: [] });
  const [personalInfo, setPersonalInfo] = useState<CardPersonalInfo[]>([]);
  // Custom field values, keyed by FieldDefinition.ID; the editor state is
  // initialized per-definition on first render (see editorFor below).
  const [customFieldValues, setCustomFieldValues] = useState<Record<string, FieldValueEditorState>>({});
  const [selectedCircles, setSelectedCircles] = useState<Circle[]>([]);
  const [newCircle, setNewCircle] = useState('');
  const [selectedTags, setSelectedTags] = useState<Tag[]>([]);
  const [newTag, setNewTag] = useState('');
  const [createBirthdayReminder, setCreateBirthdayReminder] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const editorFor = (definition: FieldDefinition): FieldValueEditorState =>
    customFieldValues[definition.id] ?? emptyEditorValue(definition);

  const handleCustomFieldChange = (definition: FieldDefinition) => (next: FieldValueEditorState) => {
    setCustomFieldValues((prev) => ({ ...prev, [definition.id]: next }));
  };

  const handleChange = (field: string) => (event: React.ChangeEvent<HTMLInputElement>) => {
    if (field === 'birthday') {
      setFormData({ ...formData, birthday: autoFormatBirthdayInput(event.target.value, formData.birthday) });
    } else if (field === 'anniversary') {
      setFormData({ ...formData, anniversary: autoFormatBirthdayInput(event.target.value, formData.anniversary) });
    } else {
      setFormData({ ...formData, [field]: event.target.value });
    }
  };

  const handleAddCircle = () => {
    const trimmed = newCircle.trim();
    if (trimmed && !selectedCircles.find(sc => sc.name === trimmed)) {
      setSelectedCircles([...selectedCircles, { id: '', created_at: '', updated_at: '', name: trimmed }]);
      setNewCircle('');
    }
  };

  const handleRemoveCircle = (circle: Circle) => {
    setSelectedCircles(selectedCircles.filter(c => c.id !== circle.id && c.name !== circle.name));
  };

  const handleAddTag = () => {
    const trimmed = newTag.trim();
    if (trimmed && !selectedTags.find(t => t.name === trimmed)) {
      setSelectedTags([...selectedTags, { id: '', created_at: '', updated_at: '', name: trimmed }]);
      setNewTag('');
    }
  };

  const handleRemoveTag = (tag: Tag) => {
    setSelectedTags(selectedTags.filter(t => t.id !== tag.id && t.name !== tag.name));
  };

  const handleSubmit = async () => {
    if (!formData.firstname.trim()) {
      setError(t('contacts.add.requiredFields'));
      return;
    }

    // Parse birthday from user's preferred format to ISO format
    let birthdayISO = '';
    if (formData.birthday.trim()) {
      const parsed = parseBirthdayInput(formData.birthday);
      if (parsed === null) {
        setError(t('contactDetail.birthdayError'));
        return;
      }
      birthdayISO = parsed;
    }

    let anniversaryISO = '';
    if (formData.anniversary.trim()) {
      const parsed = parseBirthdayInput(formData.anniversary);
      if (parsed === null) {
        setError(t('contactDetail.birthdayError'));
        return;
      }
      anniversaryISO = parsed;
    }

    setLoading(true);
    setError('');

    try {
      const nameComponents: NameComponent[] = [];
      if (formData.prefix.trim()) nameComponents.push({ kind: 'title', value: formData.prefix.trim() });
      nameComponents.push({ kind: 'given', value: formData.firstname.trim() });
      if (formData.middle_name.trim()) nameComponents.push({ kind: 'given2', value: formData.middle_name.trim() });
      if (formData.lastname.trim()) nameComponents.push({ kind: 'surname', value: formData.lastname.trim() });
      if (formData.suffix.trim()) nameComponents.push({ kind: 'generation', value: formData.suffix.trim() });

      const cardEmails = valuesToCardEmails(emails);
      const cardPhones = valuesToCardPhones(phones);
      const cardLinks = valuesToCardLinks(urls);
      const cardImpp = valuesToCardImpp(impps);
      const cardAddresses = valuesToCardAddresses(addresses);
      const anniversaries = withAnniversary(withAnniversary(undefined, 'birth', birthdayISO), 'wedding', anniversaryISO);
      const organizations = withOrganization(formData.organization, formData.department);
      const titles = withTitles(formData.job_title, formData.role);

      const newRecord = await createContactRecord({
        gender: formData.gender,
        card: {
          kind: formData.cardKind || undefined,
          language: formData.language || undefined,
          name: { components: nameComponents },
          nicknames: formData.nickname.trim() ? [{ name: formData.nickname.trim() }] : undefined,
          emails: cardEmails.length > 0 ? cardEmails : undefined,
          phones: cardPhones.length > 0 ? cardPhones : undefined,
          links: cardLinks.length > 0 ? cardLinks : undefined,
          imppAddresses: cardImpp.length > 0 ? cardImpp : undefined,
          addresses: cardAddresses.length > 0 ? cardAddresses : undefined,
          anniversaries: anniversaries.length > 0 ? anniversaries : undefined,
          speakToAs:
            speakToAs.pronouns?.length || speakToAs.grammaticalGenders?.length
              ? speakToAs
              : undefined,
          personalInfo: personalInfo.length > 0 ? personalInfo : undefined,
          organizations: organizations.length > 0 ? organizations : undefined,
          titles: titles.length > 0 ? titles : undefined,
        },
        crm: {
          kind: formData.kind,
          how_we_met: formData.how_we_met,
          work_information: formData.work_information,
          contact_information: formData.contact_information,
        },
      });

      // Custom field values (v2): the contact must exist before values can
      // attach to it, so they are set after creation via the nested
      // full-replace endpoint.
      const fieldValueInputs: FieldValueInput[] = [];
      for (const definition of fieldDefinitions) {
        const editor = customFieldValues[definition.id] ?? emptyEditorValue(definition);
        if (!isEditorValueEmpty(definition, editor)) {
          fieldValueInputs.push({
            field_definition_id: definition.id,
            value: editorToWireValue(definition, editor),
          });
        }
      }
      if (fieldValueInputs.length > 0) {
        await replaceContactFieldValues(newRecord.id, fieldValueInputs);
      }

      if (createBirthdayReminder && birthdayISO) {
        let day: number | undefined;
        let month: number | undefined;

        if (birthdayISO.startsWith('--')) {
          month = parseInt(birthdayISO.substring(2, 4), 10) - 1;
          day = parseInt(birthdayISO.substring(5, 7), 10);
        } else {
          const parts = birthdayISO.split('-');
          if (parts.length === 3) {
            month = parseInt(parts[1], 10) - 1;
            day = parseInt(parts[2], 10);
          }
        }

        if (day !== undefined && month !== undefined && !isNaN(day) && !isNaN(month)) {
          const today = new Date();
          let nextBirthday = new Date(today.getFullYear(), month, day);

          if (nextBirthday < today) {
            nextBirthday.setFullYear(today.getFullYear() + 1);
          }

          nextBirthday.setHours(9, 0, 0, 0);

          await createReminder(newRecord.id, {
            message: t('reminders.birthdayMessage', { name: `${formData.firstname} ${formData.lastname}` }),
            by_mail: true,
            remind_at: nextBirthday.toISOString(),
            recurrence: 'yearly',
            reoccur_from_completion: false,
            contact_id: newRecord.id
          });
        }
      }

      // Add circle memberships for selected circles
      for (const circle of selectedCircles) {
        try {
          let circleId = circle.id;
          if (!circleId) {
            const created = await createCircle(circle.name);
            circleId = created.circle.id;
          }
          await addCircleMember(circleId, newRecord.uid);
        } catch {
          // Silently skip — the contact exists, memberships are best-effort
        }
      }

      // Add tag memberships for selected tags
      for (const tag of selectedTags) {
        try {
          let tagId = tag.id;
          if (!tagId) {
            const created = await createTag(tag.name);
            tagId = created.tag.id;
          }
          await addContactTag(newRecord.uid, tagId);
        } catch {
          // Silently skip — the contact exists, memberships are best-effort
        }
      }

      onContactAdded(newRecord.id);
      showSuccess(t('contacts.add.success'));
      handleClose();
    } catch (err) {
      handleError(err, { operation: 'creating contact' }, { showError });
      const errorMessage = getErrorMessage(err);
      setError(errorMessage);
    } finally {
      setLoading(false);
    }
  };

  const handleClose = () => {
    setFormData({ ...emptyForm, language: defaultLanguage() });
    setEmails([]);
    setPhones([]);
    setAddresses([]);
    setUrls([]);
    setImpps([]);
    setSpeakToAs({ grammaticalGenders: [], pronouns: [] });
    setPersonalInfo([]);
    setCustomFieldValues({});
    setSelectedCircles([]);
    setNewCircle('');
    setSelectedTags([]);
    setNewTag('');
    setCreateBirthdayReminder(false);
    setError('');
    onClose();
  };

  return (
    <AppDialog open={open} onClose={handleClose} maxWidth="md" fullWidth>
      <DialogTitle>{t('contacts.add.title')}</DialogTitle>
      <DialogContent>
        {error && (
          <Typography color="error" sx={{ mb: 2 }}>
            {error}
          </Typography>
        )}
        <Stack spacing={2} sx={{ mt: 1 }}>
          <SectionHeading label={t('contactDetail.section.name')} />
          {isOn('prefix') && (
            <TextField label={t('contacts.prefix')} fullWidth value={formData.prefix} onChange={handleChange('prefix')} />
          )}
          <Stack direction="row" spacing={2}>
            <TextField
              label={t('contacts.firstname')}
              fullWidth
              value={formData.firstname}
              onChange={handleChange('firstname')}
              required
            />
            {isOn('middle_name') && (
              <TextField label={t('contacts.middleName')} fullWidth value={formData.middle_name} onChange={handleChange('middle_name')} />
            )}
            <TextField
              label={t('contacts.lastname')}
              fullWidth
              value={formData.lastname}
              onChange={handleChange('lastname')}
            />
          </Stack>
          {isOn('suffix') && (
            <TextField label={t('contacts.suffix')} fullWidth value={formData.suffix} onChange={handleChange('suffix')} />
          )}
          {isOn('nickname') && (
            <TextField label={t('contacts.nickname')} fullWidth value={formData.nickname} onChange={handleChange('nickname')} />
          )}

          <SectionHeading label={t('contactDetail.section.metadata')} />
          <TextField
            select
            label={t('contacts.kind')}
            fullWidth
            value={formData.kind}
            onChange={handleChange('kind')}
          >
            <MenuItem value="human">{t('contacts.human')}</MenuItem>
            <MenuItem value="animal">{t('contacts.animal')}</MenuItem>
          </TextField>
          {isOn('cardKind') && (
            <TextField
              select
              label={t('contacts.cardKindLabel')}
              fullWidth
              value={formData.cardKind}
              onChange={handleChange('cardKind')}
            >
              <MenuItem value="">
                <em>{t('common.none')}</em>
              </MenuItem>
              {(['individual', 'group', 'org', 'location', 'application', 'device'] as const).map((opt) => (
                <MenuItem key={opt} value={opt}>
                  {t(`contacts.cardKind.${opt}`)}
                </MenuItem>
              ))}
            </TextField>
          )}
          {isOn('language') && (
            <LanguageField
              value={formData.language}
              onChange={(v) => handleChange('language')({ target: { value: v } } as any)}
              fullWidth
            />
          )}

          {sectionEnabled(['birthday', 'anniversary', 'personalInfo']) && (
            <SectionHeading label={t('contactDetail.section.about')} />
          )}
          {isOn('birthday') && (
            <>
              <TextField
                label={t('contacts.birthday')}
                fullWidth
                value={formData.birthday}
                onChange={handleChange('birthday')}
                placeholder={getBirthdayPlaceholder()}
                helperText={t('contacts.birthdayFormat')}
              />
              {formData.birthday && (
                <FormControlLabel
                  control={
                    <Switch
                      checked={createBirthdayReminder}
                      onChange={(e) => setCreateBirthdayReminder(e.target.checked)}
                    />
                  }
                  label={t('contacts.add.createBirthdayReminder')}
                />
              )}
            </>
          )}
          {isOn('anniversary') && (
            <TextField
              label={t('contacts.anniversary')}
              fullWidth
              value={formData.anniversary}
              onChange={handleChange('anniversary')}
              placeholder={getBirthdayPlaceholder()}
              helperText={t('contacts.birthdayFormat')}
            />
          )}
          {isOn('personalInfo') && (
            <PersonalInfoEditor label={t('contacts.personalInfoLabel')} value={personalInfo} onChange={setPersonalInfo} />
          )}

          {sectionEnabled(['phones', 'addresses', 'emails', 'imppAddresses', 'links']) && (
            <SectionHeading label={t('contactDetail.section.contact')} />
          )}
          {isOn('phones') && (
            <MultiValueField label={t('contacts.phone')} value={phones} onChange={setPhones} valueType="tel" defaultType="cell" />
          )}
          {isOn('addresses') && (
            <AddressFields label={t('contacts.address')} value={addresses} onChange={setAddresses} />
          )}
          {isOn('emails') && (
            <MultiValueField label={t('contacts.email')} value={emails} onChange={setEmails} valueType="email" defaultType="home" />
          )}
          {isOn('imppAddresses') && (
            <MultiValueField label={t('contacts.impps')} value={impps} onChange={setImpps} defaultType="" freeTextType />
          )}
          {isOn('links') && (
            <MultiValueField label={t('contacts.urls')} value={urls} onChange={setUrls} valueType="url" defaultType="home" />
          )}

          {sectionEnabled(['speakToAs', 'gender']) && (
            <SectionHeading label={t('contactDetail.section.genderAndPronouns')} />
          )}
          {isOn('speakToAs') && (
            <SpeakToAsEditor value={speakToAs} onChange={setSpeakToAs} />
          )}
          {isOn('gender') && (
            <Autocomplete
              fullWidth
              freeSolo
              options={[...GENDER_OPTIONS]}
              getOptionLabel={(v) => GENDER_OPTIONS.includes(v as (typeof GENDER_OPTIONS)[number]) ? t(`contacts.${v}`) : (v || '')}
              value={formData.gender || null}
              onChange={(_, v) => handleChange('gender')({ target: { value: v || '' } } as any)}
              onInputChange={(_, v) => handleChange('gender')({ target: { value: v } } as any)}
              renderInput={(params) => (
                <TextField {...params} label={t('contacts.gender')} fullWidth placeholder={t('contacts.selectGender')} />
              )}
            />
          )}

          {sectionEnabled(['organizations', 'titles', 'work_information']) && (
            <SectionHeading label={t('contactDetail.section.professional')} />
          )}
          {isOn('organizations') && (
            <>
              <TextField label={t('contacts.organization')} fullWidth value={formData.organization} onChange={handleChange('organization')} />
              <TextField label={t('contacts.department')} fullWidth value={formData.department} onChange={handleChange('department')} />
            </>
          )}
          {isOn('titles') && (
            <>
              <TextField label={t('contacts.jobTitle')} fullWidth value={formData.job_title} onChange={handleChange('job_title')} />
              <TextField label={t('contacts.role')} fullWidth value={formData.role} onChange={handleChange('role')} />
            </>
          )}
          {isOn('work_information') && (
            <TextField
              label={t('contacts.workInformation')}
              fullWidth
              multiline
              rows={2}
              value={formData.work_information}
              onChange={handleChange('work_information')}
            />
          )}

          {sectionEnabled(['how_we_met', 'contact_information']) && (
            <SectionHeading label={t('contactDetail.section.notes')} />
          )}
          {isOn('how_we_met') && (
            <TextField
              label={t('contacts.howWeMet')}
              fullWidth
              multiline
              rows={2}
              value={formData.how_we_met}
              onChange={handleChange('how_we_met')}
            />
          )}
          {isOn('contact_information') && (
            <TextField
              label={t('contacts.contactInformation')}
              fullWidth
              multiline
              rows={2}
              value={formData.contact_information}
              onChange={handleChange('contact_information')}
            />
          )}

          {/* Custom Fields (v2): one per-type editor per definition */}
          {fieldDefinitions.map((definition) => (
            <Box key={definition.id}>
              <Typography variant="subtitle2" gutterBottom>
                {definition.label}
              </Typography>
              <FieldValueEditor
                definition={definition}
                value={editorFor(definition)}
                onChange={handleCustomFieldChange(definition)}
              />
            </Box>
          ))}
          <Box>
            <Typography variant="subtitle2" gutterBottom>
              {t('contacts.circles')}
            </Typography>
            <Box sx={{ display: 'flex', gap: 1, mb: 1, flexWrap: 'wrap' }}>
              {selectedCircles.map(c => (
                <Chip
                  key={c.id || c.name}
                  label={c.name}
                  onDelete={() => handleRemoveCircle(c)}
                  color="primary"
                  size="small"
                />
              ))}
            </Box>
            <Stack direction="row" spacing={1}>
              <TextField
                select
                label={t('contacts.selectCircle')}
                fullWidth
                value=""
                onChange={(e) => {
                  const value = e.target.value;
                  if (value) {
                    const circle = availableCircles.find(c => c.name === value);
                    if (circle && !selectedCircles.find(sc => sc.id === circle.id || sc.name === circle.name)) {
                      setSelectedCircles([...selectedCircles, circle]);
                    }
                  }
                }}
              >
                <MenuItem value="">{t('contacts.selectCircle')}</MenuItem>
                {availableCircles
                  .filter(c => !selectedCircles.find(sc => sc.id === c.id || sc.name === c.name))
                  .map(c => (
                    <MenuItem key={c.id || c.name} value={c.name}>
                      {c.name}
                    </MenuItem>
                  ))}
              </TextField>
              <TextField
                label={t('contacts.newCircle')}
                value={newCircle}
                onChange={(e) => setNewCircle(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    e.preventDefault();
                    handleAddCircle();
                  }
                }}
                sx={{ minWidth: 150 }}
              />
              <Button onClick={handleAddCircle} variant="outlined">
                {t('contacts.add.addCircle')}
              </Button>
            </Stack>
          </Box>
          <Box>
            <Typography variant="subtitle2" gutterBottom>
              {t('contacts.tags')}
            </Typography>
            <Box sx={{ display: 'flex', gap: 1, mb: 1, flexWrap: 'wrap' }}>
              {selectedTags.map(t => (
                <Chip
                  key={t.id || t.name}
                  label={t.name}
                  onDelete={() => handleRemoveTag(t)}
                  color="secondary"
                  size="small"
                />
              ))}
            </Box>
            <Stack direction="row" spacing={1}>
              <TextField
                select
                label={t('contacts.selectTag')}
                fullWidth
                value=""
                onChange={(e) => {
                  const value = e.target.value;
                  if (value) {
                    const tag = availableTags.find(t => t.name === value);
                    if (tag && !selectedTags.find(st => st.id === tag.id || st.name === tag.name)) {
                      setSelectedTags([...selectedTags, tag]);
                    }
                  }
                }}
              >
                <MenuItem value="">{t('contacts.selectTag')}</MenuItem>
                {availableTags
                  .filter(t => !selectedTags.find(st => st.id === t.id || st.name === t.name))
                  .map(t => (
                    <MenuItem key={t.id || t.name} value={t.name}>
                      {t.name}
                    </MenuItem>
                  ))}
              </TextField>
              <TextField
                label={t('contacts.newTag')}
                value={newTag}
                onChange={(e) => setNewTag(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    e.preventDefault();
                    handleAddTag();
                  }
                }}
                sx={{ minWidth: 150 }}
              />
              <Button onClick={handleAddTag} variant="outlined">
                {t('common.add')}
              </Button>
            </Stack>
          </Box>
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={handleClose} disabled={loading}>
          {t('common.cancel')}
        </Button>
        <Button onClick={handleSubmit} variant="contained" disabled={loading}>
          {loading ? t('common.saving') : t('contacts.add.create')}
        </Button>
      </DialogActions>
    </AppDialog>
  );
}
