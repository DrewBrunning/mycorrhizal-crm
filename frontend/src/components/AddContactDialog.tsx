import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  TextField,
  MenuItem,
  Chip,
  Box,
  Typography,
  Stack,
  Divider,
} from '@mui/material';
import AppDialog from './AppDialog';
import MultiValueField from './MultiValueField';
import LanguageField from './LanguageField';
import {
  createContactRecord,
  ContactValue,
  NameComponent,
  valuesToCardEmails,
  valuesToCardPhones,
} from '../api/contacts';
import { Circle } from '../api/circles';
import { addCircleMember, createCircle } from '../api/circles';
import { Tag } from '../api/tags';
import { createTag, addContactTag } from '../api/tags';
import { useSnackbar } from '../context/SnackbarContext';
import { handleError, getErrorMessage } from '../utils/errorHandler';
import { ContactFieldKey, resolveEnabledFields } from '../contactFields';
import i18n from '../i18n/config';

interface AddContactDialogProps {
  open: boolean;
  onClose: () => void;
  onContactAdded: (contactId: number) => void;
  availableCircles: Circle[];
  availableTags: Tag[];
  enabledFields?: Set<ContactFieldKey>;
}

const emptyForm = {
  firstname: '',
  lastname: '',
  prefix: '',
  middle_name: '',
  suffix: '',
  nickname: '',
  kind: 'human',
  language: '',
};

export default function AddContactDialog({
  open,
  onClose,
  onContactAdded,
  availableCircles,
  availableTags,
  enabledFields
}: AddContactDialogProps) {
  const { t } = useTranslation();
  const { showError, showSuccess } = useSnackbar();
  const enabled = enabledFields ?? resolveEnabledFields(null);
  const isOn = (key: ContactFieldKey) => enabled.has(key);

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
  const [selectedCircles, setSelectedCircles] = useState<Circle[]>([]);
  const [newCircle, setNewCircle] = useState('');
  const [selectedTags, setSelectedTags] = useState<Tag[]>([]);
  const [newTag, setNewTag] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const handleChange = (field: string) => (event: React.ChangeEvent<HTMLInputElement>) => {
    setFormData({ ...formData, [field]: event.target.value });
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

      const newRecord = await createContactRecord({
        card: {
          language: formData.language || undefined,
          name: { components: nameComponents },
          nicknames: formData.nickname.trim() ? [{ name: formData.nickname.trim() }] : undefined,
          emails: cardEmails.length > 0 ? cardEmails : undefined,
          phones: cardPhones.length > 0 ? cardPhones : undefined,
        },
        crm: {
          kind: formData.kind,
        },
      });

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
    setSelectedCircles([]);
    setNewCircle('');
    setSelectedTags([]);
    setNewTag('');
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
          {isOn('language') && (
            <LanguageField
              value={formData.language}
              onChange={(v) => handleChange('language')({ target: { value: v } } as any)}
              fullWidth
            />
          )}

          <SectionHeading label={t('contactDetail.section.contact')} />
          {isOn('phones') && (
            <MultiValueField label={t('contacts.phone')} value={phones} onChange={setPhones} valueType="tel" defaultType="cell" />
          )}
          {isOn('emails') && (
            <MultiValueField label={t('contacts.email')} value={emails} onChange={setEmails} valueType="email" defaultType="home" />
          )}

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
