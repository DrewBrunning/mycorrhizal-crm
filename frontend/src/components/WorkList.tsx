import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import { Box, Button, Chip, Divider, IconButton, Paper, Stack, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type {
  CardEntryPeriod,
  CardOrganization,
  CardTemporalRange,
  CardTitle,
} from '../api/contacts';

interface WorkListProps {
  organizations: CardOrganization[];
  titles: CardTitle[];
  periods?: CardEntryPeriod[];
  workInformation?: string;
  // Visibility follows the enabled-contact-fields settings; each subsection
  // is independently gateable.
  showOrganizations: boolean;
  showTitles: boolean;
  showWorkInformation: boolean;
  onAddOrganization: () => void;
  onEditOrganization: (organization: CardOrganization) => void;
  onDeleteOrganization: (organization: CardOrganization) => void;
  onAddTitle: () => void;
  onEditTitle: (title: CardTitle) => void;
  onDeleteTitle: (title: CardTitle) => void;
  onEditWorkInformation: () => void;
}

function periodFor(
  periods: CardEntryPeriod[] | undefined,
  kind: 'organization' | 'title',
  entryId: string | undefined,
): CardTemporalRange | undefined {
  if (!entryId) return undefined;
  return periods?.find((p) => p.kind === kind && p.entry_id === entryId)?.range;
}

// Work (issue #1729): organizations, job titles/roles and the CRM's free-text
// work information, moved out of the contact edit form into their own
// contact-page section. Periods are still CRM-side (ADR 0025), attached to
// the neutral entry's stable ID.
export default function WorkList({
  organizations,
  titles,
  periods,
  workInformation,
  showOrganizations,
  showTitles,
  showWorkInformation,
  onAddOrganization,
  onEditOrganization,
  onDeleteOrganization,
  onAddTitle,
  onEditTitle,
  onDeleteTitle,
  onEditWorkInformation,
}: WorkListProps) {
  const { t } = useTranslation();

  const confirmDelete = (action: () => void) => {
    if (window.confirm(t('contactDetail.confirmDelete'))) action();
  };

  const formatPeriod = (range: CardTemporalRange | undefined): string | null => {
    const start = range?.start?.year != null ? String(range.start.year) : '';
    const end = range?.end?.year != null ? String(range.end.year) : '';
    if (start && end) return `${start} – ${end}`;
    if (start) return t('contactDetail.period.since', { year: start });
    if (end) return t('contactDetail.period.until', { year: end });
    return null;
  };

  const renderOrganization = (organization: CardOrganization) => {
    const department = organization.units?.[0]?.name;
    const period = formatPeriod(periodFor(periods, 'organization', organization.id));
    return (
      <Paper
        key={organization.id ?? organization.name}
        variant="outlined"
        sx={{
          p: 1.5,
          '&:hover .work-actions, &:focus-within .work-actions': { opacity: 1 },
        }}
      >
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
          <Box sx={{ minWidth: 0, flex: 1 }}>
            <Typography variant="body1" sx={{ overflowWrap: 'anywhere' }}>
              {organization.name}
            </Typography>
            <Box
              sx={{ display: 'flex', alignItems: 'center', gap: 0.75, flexWrap: 'wrap', mt: 0.25 }}
            >
              {department && <Chip size="small" label={department} />}
              {period && (
                <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                  {period}
                </Typography>
              )}
            </Box>
          </Box>
          <Box
            className="work-actions"
            sx={{ display: 'flex', gap: 0.5, opacity: 0, transition: 'opacity 0.2s' }}
          >
            <IconButton
              size="small"
              onClick={() => onEditOrganization(organization)}
              aria-label={t('common.edit')}
            >
              <EditIcon fontSize="small" />
            </IconButton>
            <IconButton
              size="small"
              color="error"
              onClick={() => confirmDelete(() => onDeleteOrganization(organization))}
              aria-label={t('common.delete')}
            >
              <DeleteIcon fontSize="small" />
            </IconButton>
          </Box>
        </Box>
      </Paper>
    );
  };

  const renderTitle = (title: CardTitle) => {
    const kind = title.kind === 'role' ? 'role' : 'title';
    const period = formatPeriod(periodFor(periods, 'title', title.id));
    return (
      <Paper
        key={title.id ?? `${kind}:${title.name}`}
        variant="outlined"
        sx={{
          p: 1.5,
          '&:hover .work-actions, &:focus-within .work-actions': { opacity: 1 },
        }}
      >
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
          <Box sx={{ minWidth: 0, flex: 1 }}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75, flexWrap: 'wrap' }}>
              <Chip
                size="small"
                label={
                  kind === 'role'
                    ? t('contactDetail.work.kindOptions.role')
                    : t('contacts.jobTitle')
                }
              />
              <Typography variant="body1" sx={{ overflowWrap: 'anywhere' }}>
                {title.name}
              </Typography>
              {period && (
                <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                  {period}
                </Typography>
              )}
            </Box>
          </Box>
          <Box
            className="work-actions"
            sx={{ display: 'flex', gap: 0.5, opacity: 0, transition: 'opacity 0.2s' }}
          >
            <IconButton
              size="small"
              onClick={() => onEditTitle(title)}
              aria-label={t('common.edit')}
            >
              <EditIcon fontSize="small" />
            </IconButton>
            <IconButton
              size="small"
              color="error"
              onClick={() => confirmDelete(() => onDeleteTitle(title))}
              aria-label={t('common.delete')}
            >
              <DeleteIcon fontSize="small" />
            </IconButton>
          </Box>
        </Box>
      </Paper>
    );
  };

  const renderHeading = (label: string, onAdd: () => void, addLabel: string, divider: boolean) => (
    <>
      {divider && <Divider sx={{ my: 1.5 }} />}
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 1 }}>
        <Typography variant="subtitle2" component="h3">
          {label}
        </Typography>
        <Button startIcon={<AddIcon />} onClick={onAdd} variant="outlined" size="small">
          {addLabel}
        </Button>
      </Box>
    </>
  );

  return (
    <Box>
      {showOrganizations && (
        <Box component="section" aria-label={t('contactDetail.work.organizations')}>
          {renderHeading(
            t('contactDetail.work.organizations'),
            onAddOrganization,
            t('contactDetail.work.addOrganization'),
            false,
          )}
          {organizations.length === 0 ? (
            <Typography variant="body2" sx={{ color: 'text.secondary', py: 1 }}>
              {t('contactDetail.work.noOrganizations')}
            </Typography>
          ) : (
            <Stack spacing={1}>{organizations.map(renderOrganization)}</Stack>
          )}
        </Box>
      )}

      {showTitles && (
        <Box component="section" aria-label={t('contactDetail.work.titles')}>
          {renderHeading(
            t('contactDetail.work.titles'),
            onAddTitle,
            t('contactDetail.work.addTitle'),
            showOrganizations,
          )}
          {titles.length === 0 ? (
            <Typography variant="body2" sx={{ color: 'text.secondary', py: 1 }}>
              {t('contactDetail.work.noTitles')}
            </Typography>
          ) : (
            <Stack spacing={1}>{titles.map(renderTitle)}</Stack>
          )}
        </Box>
      )}

      {showWorkInformation && (
        <Box component="section" aria-label={t('contactDetail.workInfo')}>
          {renderHeading(
            t('contactDetail.workInfo'),
            onEditWorkInformation,
            t('contactDetail.work.editWorkInformation'),
            showOrganizations || showTitles,
          )}
          {workInformation ? (
            <Typography variant="body2" sx={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>
              {workInformation}
            </Typography>
          ) : (
            <Typography variant="body2" sx={{ color: 'text.secondary' }}>
              {t('contactDetail.work.noWorkInformation')}
            </Typography>
          )}
        </Box>
      )}
    </Box>
  );
}
