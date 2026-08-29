import {
  Autocomplete,
  Box,
  Button,
  Chip,
  CircularProgress,
  DialogActions,
  DialogContent,
  DialogTitle,
  TextField,
} from '@mui/material';
import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { type Contact, getContacts } from '../api/contacts';
import { handleFetchError } from '../utils/errorHandler';
import AppDialog from './AppDialog';

interface ContactBrief {
  ID: number;
  firstname: string;
  lastname: string;
  nickname?: string;
}

function toContactBrief(c: Contact): ContactBrief {
  return { ID: c.ID, firstname: c.firstname, lastname: c.lastname, nickname: c.nickname };
}

function contactLabel(contact: ContactBrief): string {
  if (contact.nickname) {
    return `${contact.firstname} "${contact.nickname}" ${contact.lastname}`;
  }
  return `${contact.firstname} ${contact.lastname}`;
}

interface AddNoteDialogProps {
  open: boolean;
  onClose: () => void;
  onSave: (content: string, date: string, contactId?: number) => Promise<void>;
  noteContactId?: number;
  noteContactName?: string;
}

export default function AddNoteDialog({
  open,
  onClose,
  onSave,
  noteContactId,
  noteContactName,
}: AddNoteDialogProps) {
  const { t } = useTranslation();
  const [content, setContent] = useState('');
  const [date, setDate] = useState(new Date().toISOString().split('T')[0]);
  const [error, setError] = useState('');
  const [saving, setSaving] = useState(false);

  const lockedContact = noteContactId != null;

  const [contacts, setContacts] = useState<ContactBrief[]>([]);
  const [contactsLoading, setContactsLoading] = useState(false);
  const [searchInput, setSearchInput] = useState('');
  const [selectedContact, setSelectedContact] = useState<ContactBrief | null>(null);

  const loadContacts = useCallback(async (search: string = '') => {
    setContactsLoading(true);
    try {
      const response = await getContacts({ limit: 40, search });
      setContacts((response.contacts || []).map(toContactBrief));
    } catch (err) {
      handleFetchError(err, 'loading contacts');
    } finally {
      setContactsLoading(false);
    }
  }, []);

  useEffect(() => {
    if (!open) return;
    const timeoutId = setTimeout(() => {
      loadContacts(searchInput);
    }, 300);
    return () => clearTimeout(timeoutId);
  }, [searchInput, open, loadContacts]);

  useEffect(() => {
    if (open) {
      loadContacts();
    }
  }, [open, loadContacts]);

  const handleSave = async () => {
    if (!content.trim()) {
      setError(t('noteDialog.required'));
      return;
    }

    setSaving(true);
    try {
      await onSave(content, date, lockedContact ? noteContactId : selectedContact?.ID);
      handleClose();
    } catch {
      setError(t('noteDialog.saveError'));
    } finally {
      setSaving(false);
    }
  };

  const handleClose = () => {
    setContent('');
    setDate(new Date().toISOString().split('T')[0]);
    setError('');
    setSelectedContact(null);
    setSearchInput('');
    setContacts([]);
    onClose();
  };

  return (
    <AppDialog open={open} onClose={handleClose} maxWidth="sm" fullWidth>
      <DialogTitle>{t('noteDialog.title')}</DialogTitle>
      <DialogContent>
        <Box sx={{ pt: 1, display: 'flex', flexDirection: 'column', gap: 2 }}>
          {lockedContact ? (
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
              <Chip label={noteContactName} size="small" color="primary" />
            </Box>
          ) : (
            <Autocomplete
              options={contacts}
              getOptionLabel={contactLabel}
              value={selectedContact}
              onChange={(_, value) => setSelectedContact(value)}
              onInputChange={(_, value, reason) => {
                if (reason === 'input') setSearchInput(value);
              }}
              loading={contactsLoading}
              filterOptions={(x) => x}
              renderInput={(params) => (
                <TextField
                  {...params}
                  label={t('notes.assignContact')}
                  placeholder={t('notes.searchContacts')}
                  slotProps={{
                    ...params.slotProps,

                    input: {
                      ...params.slotProps.input,
                      endAdornment: (
                        <>
                          {contactsLoading ? <CircularProgress color="inherit" size={20} /> : null}
                          {params.slotProps.input.endAdornment}
                        </>
                      ),
                    },
                  }}
                />
              )}
              isOptionEqualToValue={(option, value) => option.ID === value.ID}
              noOptionsText={searchInput ? t('notes.noContactsFound') : t('notes.typeToSearch')}
            />
          )}
          <TextField
            label={t('noteDialog.content')}
            placeholder={t('noteDialog.contentPlaceholder')}
            multiline
            rows={4}
            value={content}
            onChange={(e) => {
              setContent(e.target.value);
              setError('');
            }}
            error={!!error}
            helperText={error}
            fullWidth
            required
            autoFocus
          />
          <TextField
            label={t('noteDialog.date')}
            type="date"
            value={date}
            onChange={(e) => setDate(e.target.value)}
            fullWidth
            slotProps={{
              inputLabel: {
                shrink: true,
              },
            }}
          />
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={handleClose} disabled={saving}>
          {t('noteDialog.cancel')}
        </Button>
        <Button onClick={handleSave} variant="contained" disabled={saving}>
          {t('noteDialog.save')}
        </Button>
      </DialogActions>
    </AppDialog>
  );
}
