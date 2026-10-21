import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import { Box, Chip, IconButton, Paper, Stack, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { type CardAnniversary, formatAnniversaryDate } from '../api/contacts';
import { useDateFormat } from '../DateFormatProvider';

interface AnniversaryListProps {
  anniversaries: CardAnniversary[];
  onEdit: (anniversary: CardAnniversary) => void;
  onDelete: (anniversary: CardAnniversary) => void;
}

// Non-birth anniversaries (issue #1729): death and wedding. Birthday stays in
// the identity edit form, so this list filters it out defensively even if a
// caller hands it the full Card array.
export default function AnniversaryList({ anniversaries, onEdit, onDelete }: AnniversaryListProps) {
  const { t } = useTranslation();
  const { formatBirthday } = useDateFormat();
  const rows = anniversaries.filter((a) => a.kind !== 'birth');

  const handleDelete = (anniversary: CardAnniversary) => {
    if (window.confirm(t('contactDetail.confirmDelete'))) onDelete(anniversary);
  };

  if (rows.length === 0) {
    return (
      <Typography variant="body2" sx={{ color: 'text.secondary', py: 2, textAlign: 'center' }}>
        {t('contactDetail.anniversaries.empty')}
      </Typography>
    );
  }

  return (
    <Stack spacing={1}>
      {rows.map((anniversary) => {
        const iso = formatAnniversaryDate(anniversary.date);
        const date = iso ? formatBirthday(iso) : undefined;
        return (
          <Paper
            key={anniversary.id ?? `${anniversary.kind}:${iso ?? ''}`}
            variant="outlined"
            sx={{
              p: 1.5,
              '&:hover .anniversary-actions, &:focus-within .anniversary-actions': {
                opacity: 1,
              },
            }}
          >
            <Box
              sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}
            >
              <Box sx={{ minWidth: 0, flex: 1 }}>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75, flexWrap: 'wrap' }}>
                  <Chip
                    size="small"
                    label={t(`contacts.anniversaryFields.kindOptions.${anniversary.kind}`)}
                  />
                  <Typography variant="body1">{date ?? '—'}</Typography>
                </Box>
                {anniversary.place && (
                  <Typography
                    variant="body2"
                    sx={{ color: 'text.secondary', mt: 0.25, overflowWrap: 'anywhere' }}
                  >
                    {anniversary.place.full ||
                      (anniversary.place.components || [])
                        .map((c) => c.value)
                        .filter(Boolean)
                        .join(', ')}
                  </Typography>
                )}
              </Box>
              <Box
                className="anniversary-actions"
                sx={{ display: 'flex', gap: 0.5, opacity: 0, transition: 'opacity 0.2s' }}
              >
                <IconButton
                  size="small"
                  onClick={() => onEdit(anniversary)}
                  aria-label={t('common.edit')}
                >
                  <EditIcon fontSize="small" />
                </IconButton>
                <IconButton
                  size="small"
                  color="error"
                  onClick={() => handleDelete(anniversary)}
                  aria-label={t('common.delete')}
                >
                  <DeleteIcon fontSize="small" />
                </IconButton>
              </Box>
            </Box>
          </Paper>
        );
      })}
    </Stack>
  );
}
