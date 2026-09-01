import {
  Box,
  Button,
  Chip,
  MenuItem,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  TextField,
  Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { RowSourceAction, SourceImportPreviewResponse } from '../../api/sourceImport';
import { resolveRowAction } from '../../hooks/useSourceImportWizard';
import SourceImportLossReport from './SourceImportLossReport';

interface Props {
  preview: SourceImportPreviewResponse;
  rowActions: Map<number, RowSourceAction>;
  onRowAction: (rowIndex: number, action: RowSourceAction) => void;
  onSetAll: (action: RowSourceAction) => void;
}

function displayName(parsed: Record<string, string>): string {
  const name = [parsed.firstname, parsed.lastname].filter(Boolean).join(' ').trim();
  return name || parsed.nickname || parsed.email || '—';
}

// The review step: one row per source contact with its add/skip/update
// decision, bulk "apply to all" controls, a totals line, and the loss report
// — shown here, before the user commits (issue #442 recommendation 3).
export default function SourceImportReview({ preview, rowActions, onRowAction, onSetAll }: Props) {
  const { t } = useTranslation();
  const { totals } = preview;

  return (
    <Box>
      <Stack
        direction={{ xs: 'column', sm: 'row' }}
        spacing={1}
        sx={{ mb: 1.5, alignItems: { sm: 'center' } }}
      >
        <Typography variant="body2" sx={{ color: 'text.secondary', flexGrow: 1 }}>
          {t('settings.sourceImport.review.summary', {
            total: preview.total_rows,
            duplicates: preview.duplicate_count,
            errors: preview.error_count,
          })}
        </Typography>
        <Typography variant="caption" sx={{ color: 'text.secondary' }}>
          {t('settings.sourceImport.review.applyAll')}
        </Typography>
        {(['add', 'update', 'skip'] as RowSourceAction[]).map((a) => (
          <Button key={a} size="small" variant="outlined" onClick={() => onSetAll(a)}>
            {t(`settings.sourceImport.action.${a}`)}
          </Button>
        ))}
      </Stack>

      <Box sx={{ overflowX: 'auto' }}>
        <Table size="small">
          <TableHead>
            <TableRow>
              <TableCell>{t('settings.sourceImport.review.colName')}</TableCell>
              <TableCell>{t('settings.sourceImport.review.colStatus')}</TableCell>
              <TableCell>{t('settings.sourceImport.review.colBrings')}</TableCell>
              <TableCell>{t('settings.sourceImport.review.colAction')}</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {preview.rows.map((row) => {
              const action = resolveRowAction(rowActions, row);
              const invalid = row.validation_errors.length > 0;
              const rel = row.related;
              const brings: string[] = [];
              if (rel.relationships)
                brings.push(
                  t('settings.sourceImport.brings.relationships', { count: rel.relationships }),
                );
              if (rel.notes)
                brings.push(t('settings.sourceImport.brings.notes', { count: rel.notes }));
              if (rel.activities)
                brings.push(
                  t('settings.sourceImport.brings.activities', { count: rel.activities }),
                );
              if (rel.reminders)
                brings.push(t('settings.sourceImport.brings.reminders', { count: rel.reminders }));
              if (rel.gifts)
                brings.push(t('settings.sourceImport.brings.gifts', { count: rel.gifts }));
              return (
                <TableRow key={row.row_index}>
                  <TableCell>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                      {displayName(row.parsed_contact)}
                      {row.has_photo && (
                        <Chip
                          size="small"
                          variant="outlined"
                          label={t('settings.sourceImport.review.photo')}
                        />
                      )}
                    </Box>
                  </TableCell>
                  <TableCell>
                    {invalid ? (
                      <Chip size="small" color="error" label={row.validation_errors[0]} />
                    ) : row.duplicate_match ? (
                      <Chip
                        size="small"
                        color="warning"
                        label={t('settings.sourceImport.review.matches', {
                          name:
                            [
                              row.duplicate_match.existing_firstname,
                              row.duplicate_match.existing_lastname,
                            ]
                              .filter(Boolean)
                              .join(' ') || t('settings.sourceImport.review.existingContact'),
                        })}
                      />
                    ) : (
                      <Chip
                        size="small"
                        color="success"
                        variant="outlined"
                        label={t('settings.sourceImport.review.new')}
                      />
                    )}
                  </TableCell>
                  <TableCell>
                    <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                      {brings.join(' · ') || '—'}
                    </Typography>
                  </TableCell>
                  <TableCell>
                    <TextField
                      select
                      size="small"
                      value={action}
                      onChange={(e) =>
                        onRowAction(row.row_index, e.target.value as RowSourceAction)
                      }
                      sx={{ minWidth: 120 }}
                    >
                      <MenuItem value="add" disabled={invalid}>
                        {t('settings.sourceImport.action.add')}
                      </MenuItem>
                      <MenuItem value="update" disabled={!row.duplicate_match}>
                        {t('settings.sourceImport.action.update')}
                      </MenuItem>
                      <MenuItem value="skip">{t('settings.sourceImport.action.skip')}</MenuItem>
                    </TextField>
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      </Box>

      <Typography variant="body2" sx={{ color: 'text.secondary', mt: 1.5, mb: 1 }}>
        {t('settings.sourceImport.review.totals', {
          relationships: totals.relationships,
          notes: totals.notes,
          activities: totals.activities,
          reminders: totals.reminders,
        })}
      </Typography>

      <SourceImportLossReport issues={preview.loss_report} />
    </Box>
  );
}
