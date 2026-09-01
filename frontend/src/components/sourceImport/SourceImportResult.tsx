import CheckCircleOutlineIcon from '@mui/icons-material/CheckCircleOutlined';
import {
  Alert,
  AlertTitle,
  Box,
  List,
  ListItem,
  ListItemText,
  Stack,
  Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { SourceImportResult as Result } from '../../api/sourceImport';

interface Props {
  result: Result;
  photosPending?: boolean;
}

// The final screen: the created/updated/skipped counts, the graph entities
// that came across, any deferred photo progress, and the named failures
// (never a generic "something went wrong").
export default function SourceImportResult({ result, photosPending }: Props) {
  const { t } = useTranslation();

  const lines: string[] = [
    t('settings.sourceImport.result.contacts', {
      created: result.created,
      updated: result.updated,
      skipped: result.skipped,
    }),
  ];
  if (result.relationships_created) lines.push(t('settings.sourceImport.brings.relationships', { count: result.relationships_created }));
  if (result.notes_created) lines.push(t('settings.sourceImport.brings.notes', { count: result.notes_created }));
  if (result.activities_created) lines.push(t('settings.sourceImport.brings.activities', { count: result.activities_created }));
  if (result.reminders_created) lines.push(t('settings.sourceImport.brings.reminders', { count: result.reminders_created }));
  if (result.gifts_created) lines.push(t('settings.sourceImport.brings.gifts', { count: result.gifts_created }));

  return (
    <Box sx={{ py: 1 }}>
      <Stack direction="row" spacing={1} sx={{ mb: 1, alignItems: 'center' }}>
        <CheckCircleOutlineIcon color="success" />
        <Typography variant="h6" component="h2">
          {t('settings.sourceImport.result.title')}
        </Typography>
      </Stack>

      <List dense>
        {lines.map((line) => (
          <ListItem key={line} disableGutters sx={{ py: 0.25 }}>
            <ListItemText primary={line} slotProps={{ primary: { variant: 'body2' } }} />
          </ListItem>
        ))}
      </List>

      {result.photos_queued > 0 && (
        <Alert severity={photosPending ? 'info' : 'success'} sx={{ py: 0, my: 1 }}>
          {photosPending
            ? t('settings.sourceImport.result.photosPending', { total: result.photos_queued })
            : t('settings.sourceImport.result.photosDone', {
                saved: result.photos_saved,
                failed: result.photos_failed,
              })}
        </Alert>
      )}

      {result.errors.length > 0 && (
        <Alert severity="warning" sx={{ mt: 1 }}>
          <AlertTitle>
            {t('settings.sourceImport.result.partialFailures', { count: result.errors.length })}
          </AlertTitle>
          <List dense disablePadding>
            {result.errors.map((err, i) => (
              <ListItem key={`${i}-${err}`} disableGutters sx={{ py: 0.25 }}>
                <ListItemText primary={err} slotProps={{ primary: { variant: 'caption' } }} />
              </ListItem>
            ))}
          </List>
        </Alert>
      )}
    </Box>
  );
}
