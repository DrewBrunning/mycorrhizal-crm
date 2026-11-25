import {
  Box,
  Button,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  ToggleButton,
  ToggleButtonGroup,
  Typography,
} from '@mui/material';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { ImportRowIssue } from '../api/import';
import { importIssuesToCsv } from '../utils/importReportCsv';
import { importWarningKey } from '../utils/importWarning';

// Issue #2187: save the skipped/failed rows as a CSV.
function downloadIssuesCsv(
  issues: ImportRowIssue[],
  statusLabel: (issue: ImportRowIssue) => string,
) {
  const blob = new Blob([importIssuesToCsv(issues, statusLabel)], {
    type: 'text/csv;charset=utf-8',
  });
  const url = window.URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = 'import-report.csv';
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
  window.URL.revokeObjectURL(url);
}

type IssueFilter = 'all' | 'notImported' | 'warnings';

// The itemized "rows not imported" report with its CSV download. Shared by the
// import wizard's result step (#2187) and the import-history report dialog
// (#2465). Renders nothing when there are no issues. A 'warning' row (#2741)
// was imported and only lost a value, so it is labelled Warning, explained by
// its concept, and kept apart from the rows that were not imported.
export default function ImportIssuesReport({ issues }: { issues: ImportRowIssue[] }) {
  const { t } = useTranslation();
  const [filter, setFilter] = useState<IssueFilter>('all');
  if (issues.length === 0) return null;

  const warningCount = issues.filter((i) => i.status === 'warning').length;
  const notImportedCount = issues.length - warningCount;
  const mixed = warningCount > 0 && notImportedCount > 0;
  const shown = !mixed
    ? issues
    : issues.filter((i) =>
        filter === 'all' ? true : (i.status === 'warning') === (filter === 'warnings'),
      );
  const title =
    warningCount === 0
      ? t('contacts.import.result.issuesTitle', 'Rows not imported')
      : notImportedCount === 0
        ? t('contacts.import.result.warningsTitle', 'Imported with warnings')
        : t('contacts.import.result.mixedTitle', 'Rows to review');

  const statusLabel = (issue: ImportRowIssue) => {
    if (issue.status === 'failed') return t('contacts.import.result.statusFailed', 'Failed');
    if (issue.status === 'warning') return t('contacts.import.result.statusWarning', 'Warning');
    return t('contacts.import.result.statusSkipped', 'Skipped');
  };
  // A warning is explained by its concept; an unknown concept (or a skipped or
  // failed row) shows the server's reason.
  const reasonLabel = (issue: ImportRowIssue) => {
    const key = issue.status === 'warning' ? importWarningKey(issue.concept) : undefined;
    return key ? t(key) : issue.reason;
  };

  return (
    <Box sx={{ mt: 2 }}>
      <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <Typography
          variant="subtitle2"
          sx={{ color: notImportedCount === 0 ? 'warning.dark' : 'error.main' }}
        >
          {title}
        </Typography>
        <Button size="small" onClick={() => downloadIssuesCsv(issues, statusLabel)}>
          {t('contacts.import.result.downloadCsv', 'Download as CSV')}
        </Button>
      </Box>
      {mixed && (
        <ToggleButtonGroup
          exclusive
          size="small"
          value={filter}
          onChange={(_, next: IssueFilter | null) => {
            if (next !== null) setFilter(next);
          }}
          aria-label={t('contacts.import.result.filterLabel', 'Show')}
          sx={{ my: 1 }}
        >
          <ToggleButton value="all">
            {t('contacts.import.result.filterAll', 'All ({{count}})', { count: issues.length })}
          </ToggleButton>
          <ToggleButton value="notImported">
            {t('contacts.import.result.filterNotImported', 'Not imported ({{count}})', {
              count: notImportedCount,
            })}
          </ToggleButton>
          <ToggleButton value="warnings">
            {t('contacts.import.result.filterWarnings', 'Warnings ({{count}})', {
              count: warningCount,
            })}
          </ToggleButton>
        </ToggleButtonGroup>
      )}
      <TableContainer sx={{ maxHeight: 260 }}>
        <Table size="small" stickyHeader aria-label={title}>
          <TableHead>
            <TableRow>
              <TableCell>{t('contacts.import.result.colRow', 'Row')}</TableCell>
              <TableCell>{t('contacts.import.result.colName', 'Contact')}</TableCell>
              <TableCell>{t('contacts.import.result.colStatus', 'Result')}</TableCell>
              <TableCell>{t('contacts.import.result.colReason', 'Reason')}</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {shown.map((issue) => (
              <TableRow key={`${issue.row}-${issue.status}-${issue.concept ?? ''}-${issue.reason}`}>
                <TableCell>{issue.row}</TableCell>
                {/* #2950: history never stores an email or phone (#2563), so say why there is no name. */}
                <TableCell>
                  {issue.name || t('contacts.import.result.nameNotKept', 'Name not kept')}
                </TableCell>
                <TableCell>{statusLabel(issue)}</TableCell>
                <TableCell>{reasonLabel(issue)}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>
    </Box>
  );
}
