import { useTranslation } from 'react-i18next';
import {
  Box,
  Typography,
  Stack,
  TextField,
  IconButton,
  Button,
  Paper,
  Autocomplete,
  MenuItem,
} from '@mui/material';
import DeleteIcon from '@mui/icons-material/Delete';
import AddIcon from '@mui/icons-material/Add';
import { CardSpeakToAs, CardGrammaticalGender, CardPronouns } from '../api/contacts';
import { useRowKeys } from '../hooks/useRowKeys';
import { CONTEXT_OPTIONS, GRAMMATICAL_GENDER_OPTIONS } from '../contactFields';

interface SpeakToAsEditorProps {
  value: CardSpeakToAs;
  onChange: (next: CardSpeakToAs) => void;
}

const EMPTY_PRONOUN: CardPronouns = { pronouns: '' };
const EMPTY_GENDER: CardGrammaticalGender = { value: '' };

export default function SpeakToAsEditor({ value, onChange }: SpeakToAsEditorProps) {
  const { t } = useTranslation();
  const genders = value.grammaticalGenders || [];
  const pronouns = value.pronouns || [];
  const genderKeys = useRowKeys(genders.length);
  const pronounKeys = useRowKeys(pronouns.length);

  const updateGender = (index: number, patch: Partial<CardGrammaticalGender>) => {
    const next = genders.map((g, i) => (i === index ? { ...g, ...patch } : g));
    onChange({ ...value, grammaticalGenders: next });
  };
  const removeGender = (index: number) => {
    genderKeys.onRemove(index);
    onChange({ ...value, grammaticalGenders: genders.filter((_, i) => i !== index) });
  };
  const addGender = () => {
    genderKeys.onAdd();
    onChange({ ...value, grammaticalGenders: [...genders, { ...EMPTY_GENDER }] });
  };

  const updatePronoun = (index: number, patch: Partial<CardPronouns>) => {
    const next = pronouns.map((p, i) => (i === index ? { ...p, ...patch } : p));
    onChange({ ...value, pronouns: next });
  };
  const removePronoun = (index: number) => {
    pronounKeys.onRemove(index);
    onChange({ ...value, pronouns: pronouns.filter((_, i) => i !== index) });
  };
  const addPronoun = () => {
    pronounKeys.onAdd();
    onChange({ ...value, pronouns: [...pronouns, { ...EMPTY_PRONOUN }] });
  };

  return (
    <Box>
      <Typography variant="subtitle2" gutterBottom>
        {t('contacts.speakToAs.pronouns')}
      </Typography>
      <Stack spacing={1} sx={{ mt: 0.5 }}>
        {pronouns.map((p, index) => (
          <Paper key={pronounKeys.keyAt(index)} variant="outlined" sx={{ p: 1.5 }}>
            <Stack spacing={1}>
              <Stack direction="row" spacing={1} alignItems="center">
                <TextField
                  label={t('contacts.speakToAs.pronouns')}
                  size="small"
                  fullWidth
                  value={p.pronouns}
                  onChange={(e) => updatePronoun(index, { pronouns: e.target.value })}
                />
                <IconButton
                  size="small"
                  color="error"
                  onClick={() => removePronoun(index)}
                  aria-label={t('common.delete')}
                >
                  <DeleteIcon fontSize="small" />
                </IconButton>
              </Stack>
              <Autocomplete
                multiple
                freeSolo
                size="small"
                options={CONTEXT_OPTIONS as readonly string[]}
                value={p.contexts || []}
                onChange={(_, newValue) => updatePronoun(index, { contexts: newValue as string[] })}
                getOptionLabel={(opt) => t(`contacts.contexts.${opt}`, opt)}
                renderInput={(params) => (
                  <TextField {...params} label={t('contacts.speakToAs.contexts')} size="small" />
                )}
              />
            </Stack>
          </Paper>
        ))}
        <Box>
          <Button size="small" startIcon={<AddIcon />} onClick={addPronoun} variant="outlined">
            {t('common.add')}
          </Button>
        </Box>
      </Stack>

      <Typography variant="subtitle2" gutterBottom sx={{ mt: 2 }}>
        {t('contacts.speakToAs.grammaticalGender')}
      </Typography>
      <Stack spacing={1} sx={{ mt: 0.5 }}>
        {genders.map((g, index) => (
          <Paper key={genderKeys.keyAt(index)} variant="outlined" sx={{ p: 1.5 }}>
            <Stack direction="row" spacing={1} alignItems="center">
              <TextField
                select
                label={t('contacts.speakToAs.grammaticalGender')}
                size="small"
                value={g.value}
                onChange={(e) => updateGender(index, { value: e.target.value })}
                sx={{ flexGrow: 1 }}
              >
                {GRAMMATICAL_GENDER_OPTIONS.map((opt) => (
                  <MenuItem key={opt} value={opt}>
                    {t(`contacts.speakToAs.gramGender.${opt}`)}
                  </MenuItem>
                ))}
              </TextField>
              <TextField
                label={t('contacts.speakToAs.language')}
                size="small"
                value={g.language || ''}
                onChange={(e) => updateGender(index, { language: e.target.value })}
              />
              <IconButton
                size="small"
                color="error"
                onClick={() => removeGender(index)}
                aria-label={t('common.delete')}
              >
                <DeleteIcon fontSize="small" />
              </IconButton>
            </Stack>
          </Paper>
        ))}
        <Box>
          <Button size="small" startIcon={<AddIcon />} onClick={addGender} variant="outlined">
            {t('common.add')}
          </Button>
        </Box>
      </Stack>
    </Box>
  );
}
