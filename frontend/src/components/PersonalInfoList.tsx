import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import LockOutlinedIcon from '@mui/icons-material/LockOutlined';
import StarIcon from '@mui/icons-material/Star';
import { Box, Chip, IconButton, Paper, Stack, Tooltip, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { CardPersonalInfo, PersonalInfoMeta } from '../api/contacts';
import { PERSONAL_INFO_KIND_OPTIONS } from '../contactFields';

interface PersonalInfoListProps {
  entries: CardPersonalInfo[];
  metaById?: Record<string, PersonalInfoMeta>;
  onEdit: (entry: CardPersonalInfo) => void;
  onDelete: (entry: CardPersonalInfo) => void;
}

// Skills & interests (issue #1729): the contact-page drill-down for
// Card.PersonalInfo, grouped by kind. Hobbies live here too (issue #1738) --
// this is the only place the web client creates them.
export default function PersonalInfoList({
  entries,
  metaById,
  onEdit,
  onDelete,
}: PersonalInfoListProps) {
  const { t } = useTranslation();

  const handleDelete = (entry: CardPersonalInfo) => {
    if (window.confirm(t('contactDetail.confirmDelete'))) onDelete(entry);
  };

  if (entries.length === 0) {
    return (
      <Typography variant="body2" sx={{ color: 'text.secondary', py: 2, textAlign: 'center' }}>
        {t('contactDetail.skills.empty')}
      </Typography>
    );
  }

  const renderItem = (entry: CardPersonalInfo) => {
    const meta = entry.id ? metaById?.[entry.id] : undefined;
    const sensitive = meta?.sensitivity && meta.sensitivity !== 'normal';
    return (
      <Paper
        key={entry.id ?? `${entry.kind}:${entry.value}`}
        variant="outlined"
        sx={{
          p: 1.5,
          '&:hover .personal-info-actions, &:focus-within .personal-info-actions': {
            opacity: 1,
          },
        }}
      >
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
          <Box sx={{ flex: 1, minWidth: 0 }}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75, flexWrap: 'wrap' }}>
              {meta?.favorite && (
                <Tooltip title={t('contactDetail.skills.favorite')}>
                  <StarIcon
                    fontSize="small"
                    color="warning"
                    aria-label={t('contactDetail.skills.favorite')}
                  />
                </Tooltip>
              )}
              <Typography variant="body1" sx={{ overflowWrap: 'anywhere' }}>
                {entry.value}
              </Typography>
              {entry.level && (
                <Chip
                  size="small"
                  label={t(`contacts.personalInfo.levelOptions.${entry.level}`, entry.level)}
                />
              )}
              {entry.label && <Chip size="small" variant="outlined" label={entry.label} />}
              {sensitive && (
                <Tooltip title={t(`preference.sensitivities.${meta.sensitivity}`)}>
                  <LockOutlinedIcon
                    fontSize="small"
                    color="action"
                    aria-label={t(`preference.sensitivities.${meta.sensitivity}`)}
                  />
                </Tooltip>
              )}
            </Box>
            {meta?.notes && (
              <Typography
                variant="body2"
                sx={{
                  color: 'text.secondary',
                  mt: 0.5,
                  whiteSpace: 'pre-wrap',
                  overflowWrap: 'anywhere',
                }}
              >
                {meta.notes}
              </Typography>
            )}
          </Box>
          <Box
            className="personal-info-actions"
            sx={{ display: 'flex', gap: 0.5, opacity: 0, transition: 'opacity 0.2s' }}
          >
            <IconButton size="small" onClick={() => onEdit(entry)} aria-label={t('common.edit')}>
              <EditIcon fontSize="small" />
            </IconButton>
            <IconButton
              size="small"
              color="error"
              onClick={() => handleDelete(entry)}
              aria-label={t('common.delete')}
            >
              <DeleteIcon fontSize="small" />
            </IconButton>
          </Box>
        </Box>
      </Paper>
    );
  };

  return (
    <Stack spacing={2}>
      {PERSONAL_INFO_KIND_OPTIONS.map((kind) => {
        const items = entries.filter((entry) => entry.kind === kind);
        if (items.length === 0) return null;
        return (
          <Box
            component="section"
            key={kind}
            aria-label={t(`contacts.personalInfo.kindOptions.${kind}`)}
          >
            <Typography
              variant="overline"
              sx={{ color: 'text.secondary', letterSpacing: 0.08, fontSize: '0.72rem' }}
            >
              {t(`contacts.personalInfo.kindOptions.${kind}`)}
            </Typography>
            <Stack spacing={1} sx={{ mt: 0.5 }}>
              {items.map(renderItem)}
            </Stack>
          </Box>
        );
      })}
      {entries.some(
        (entry) => !(PERSONAL_INFO_KIND_OPTIONS as readonly string[]).includes(entry.kind),
      ) && (
        <Box component="section" aria-label={t('contactDetail.skills.other')}>
          <Typography
            variant="overline"
            sx={{ color: 'text.secondary', letterSpacing: 0.08, fontSize: '0.72rem' }}
          >
            {t('contactDetail.skills.other')}
          </Typography>
          <Stack spacing={1} sx={{ mt: 0.5 }}>
            {entries
              .filter(
                (entry) => !(PERSONAL_INFO_KIND_OPTIONS as readonly string[]).includes(entry.kind),
              )
              .map(renderItem)}
          </Stack>
        </Box>
      )}
    </Stack>
  );
}
