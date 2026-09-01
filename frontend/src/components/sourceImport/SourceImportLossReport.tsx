import ReportProblemOutlinedIcon from '@mui/icons-material/ReportProblemOutlined';
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Alert,
  Box,
  Chip,
  List,
  ListItem,
  ListItemText,
  Typography,
} from '@mui/material';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import type { SourceImportIssue, SourceImportIssueCategory } from '../../api/sourceImport';

// The order categories are shown in — most consequential first.
const CATEGORY_ORDER: SourceImportIssueCategory[] = [
  'unsupported',
  'invalid',
  'lossy',
  'transformed',
  'skipped',
];

interface Props {
  issues: SourceImportIssue[];
}

// The pre-confirm loss report (issue #442): what the import cannot carry,
// grouped by category, shown before the user commits and again on the result
// screen. An empty list renders a reassuring "nothing lost" note.
export default function SourceImportLossReport({ issues }: Props) {
  const { t } = useTranslation();

  const grouped = useMemo(() => {
    const map = new Map<SourceImportIssueCategory, SourceImportIssue[]>();
    for (const issue of issues) {
      const list = map.get(issue.category) ?? [];
      list.push(issue);
      map.set(issue.category, list);
    }
    return CATEGORY_ORDER.filter((c) => map.has(c)).map((c) => ({
      category: c,
      items: map.get(c) as SourceImportIssue[],
    }));
  }, [issues]);

  if (issues.length === 0) {
    return (
      <Alert severity="success" sx={{ py: 0 }}>
        {t('settings.sourceImport.lossReport.none')}
      </Alert>
    );
  }

  return (
    <Box>
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1 }}>
        <ReportProblemOutlinedIcon fontSize="small" color="warning" />
        <Typography variant="subtitle2" component="h3">
          {t('settings.sourceImport.lossReport.title')}
        </Typography>
        <Chip size="small" color="warning" variant="outlined" label={issues.length} />
      </Box>
      <Typography variant="body2" sx={{ color: 'text.secondary', mb: 1 }}>
        {t('settings.sourceImport.lossReport.description')}
      </Typography>
      {grouped.map(({ category, items }) => (
        <Accordion key={category} disableGutters>
          <AccordionSummary expandIcon={<ExpandMoreIcon />}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
              <Typography variant="body2" sx={{ fontWeight: 500 }}>
                {t(`settings.sourceImport.lossCategory.${category}`)}
              </Typography>
              <Chip size="small" label={items.length} />
            </Box>
          </AccordionSummary>
          <AccordionDetails sx={{ pt: 0 }}>
            <List dense disablePadding>
              {items.map((issue, i) => (
                <ListItem key={`${issue.record}-${issue.field}-${i}`} disableGutters sx={{ py: 0.25 }}>
                  <ListItemText
                    primary={issue.message}
                    secondary={
                      issue.field ? `${issue.record} · ${issue.field}` : issue.record
                    }
                    slotProps={{
                      primary: { variant: 'body2' },
                      secondary: { variant: 'caption' },
                    }}
                  />
                </ListItem>
              ))}
            </List>
          </AccordionDetails>
        </Accordion>
      ))}
    </Box>
  );
}
