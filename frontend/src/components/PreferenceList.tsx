import { Box, Typography, IconButton, Stack, Paper, Chip } from '@mui/material';
import EditIcon from '@mui/icons-material/Edit';
import DeleteIcon from '@mui/icons-material/Delete';
import { useTranslation } from 'react-i18next';
import { Preference } from '../api/preferences';

interface PreferenceListProps {
  preferences: Preference[];
  onEdit: (preference: Preference) => void;
  onDelete: (id: string) => void;
}

// The Preferences tab is split into the two display groups the dialog offers:
// "Food & Drink Preferences" (categories food + drink) and "Media
// Preferences" (category media). Anything else (legacy categories, free-typed
// ones) falls through to an "Other" section so no data ever hides.
const FOOD_DRINK_CATEGORIES = ['food', 'drink'];
const MEDIA_CATEGORY = 'media';

export default function PreferenceList({ preferences, onEdit, onDelete }: PreferenceListProps) {
  const { t } = useTranslation();

  if (preferences.length === 0) {
    return (
      <Typography variant="body2" color="text.secondary" sx={{ py: 2, textAlign: 'center' }}>
        {t('preference.empty')}
      </Typography>
    );
  }

  const foodDrink = preferences.filter((p) => FOOD_DRINK_CATEGORIES.includes(p.category));
  const media = preferences.filter((p) => p.category === MEDIA_CATEGORY);
  const other = preferences.filter((p) => !FOOD_DRINK_CATEGORIES.includes(p.category) && p.category !== MEDIA_CATEGORY);

  const renderItem = (pref: Preference) => (
    <Paper
      key={pref.id}
      variant="outlined"
      sx={{
        p: 2,
        '&:hover .preference-actions': { opacity: 1 },
      }}
    >
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
        <Box sx={{ flex: 1 }}>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 0.5, flexWrap: 'wrap' }}>
            <Chip
              label={t(`preference.categories.${pref.category}`, pref.category)}
              size="small"
            />
            {pref.key && (
              <Chip
                label={t(`preference.keys.${pref.key}`, pref.key)}
                size="small"
              />
            )}
            {pref.sensitivity !== 'normal' && (
              <Chip
                label={t(`preference.sensitivities.${pref.sensitivity}`)}
                size="small"
                sx={{ height: 18 }}
              />
            )}
          </Box>
          <Typography variant="body1" sx={{ overflowWrap: 'anywhere' }}>{pref.value}</Typography>
        </Box>
        <Box
          className="preference-actions"
          sx={{ display: 'flex', gap: 0.5, opacity: 0, transition: 'opacity 0.2s ease-in-out' }}
        >
          <IconButton size="small" onClick={() => onEdit(pref)} aria-label={t('common.edit')}>
            <EditIcon fontSize="small" />
          </IconButton>
          <IconButton
            size="small"
            color="error"
            onClick={() => onDelete(pref.id)}
            aria-label={t('common.delete')}
          >
            <DeleteIcon fontSize="small" />
          </IconButton>
        </Box>
      </Box>
    </Paper>
  );

  const renderSection = (label: string, items: Preference[]) => {
    if (items.length === 0) return null;
    return (
      <Box>
        <Typography variant="overline" color="text.secondary" sx={{ letterSpacing: 0.08, fontSize: '0.72rem' }}>
          {label}
        </Typography>
        <Stack spacing={1.5} sx={{ mt: 0.5 }}>
          {items.map(renderItem)}
        </Stack>
      </Box>
    );
  };

  return (
    <Stack spacing={2.5}>
      {renderSection(t('preference.sections.foodDrink'), foodDrink)}
      {renderSection(t('preference.sections.media'), media)}
      {renderSection(t('preference.sections.other'), other)}
    </Stack>
  );
}
