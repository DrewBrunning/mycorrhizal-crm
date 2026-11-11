import {
  Box,
  Button,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { ImportRowIssue } from '../api/import';
import { importIssuesToCsv } from '../utils/importReportCsv';

// Issue #2187: save the skipped/failed rows as a CSV.
function downloadIssuesCsv(issues: ImportRowIssue[]) {
  const blob = new Blob([importIssuesToCsv(issues)], { type: 'text/csv;charset=utf-8' });
  const url = window.URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = 'import-report.csv';
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
  window.URL.revokeObjectURL(url);
}

// The itemized "rows not imported" report with its CSV download. Shared by the
// import wizard's result step (#2187) and the import-history report dialog
// (#2465). Renders nothing when there are no issues.
export default function ImportIssuesReport({ issues }: { issues: ImportRowIssue[] }) {
  const { t } = useTranslation();
  if (issues.length === 0) return null;

  return (
    <Box sx={{ mt: 2 }}>
      <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <Typography variant="subtitle2" color="error">
          {t('contacts.import.result.issuesTitle', 'Rows not imported')}
        </Typography>
        <Button size="small" onClick={() => downloadIssuesCsv(issues)}>
          {t('contacts.import.result.downloadCsv', 'Download as CSV')}
        </Button>
      </Box>
      <TableContainer sx={{ maxHeight: 260 }}>
        <Table
          size="small"
          stickyHeader
          aria-label={t('contacts.import.result.issuesTitle', 'Rows not imported')}
        >
          <TableHead>
            <TableRow>
              <TableCell>{t('contacts.import.result.colRow', 'Row')}</TableCell>
              <TableCell>{t('contacts.import.result.colName', 'Contact')}</TableCell>
              <TableCell>{t('contacts.import.result.colStatus', 'Result')}</TableCell>
              <TableCell>{t('contacts.import.result.colReason', 'Reason')}</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {issues.map((issue) => (
              <TableRow key={`${issue.row}-${issue.status}-${issue.reason}`}>
                <TableCell>{issue.row}</TableCell>
                <TableCell>{issue.name || '—'}</TableCell>
                <TableCell>
                  {issue.status === 'failed'
                    ? t('contacts.import.result.statusFailed', 'Failed')
                    : t('contacts.import.result.statusSkipped', 'Skipped')}
                </TableCell>
                <TableCell>{issue.reason}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>
    </Box>
  );
}
