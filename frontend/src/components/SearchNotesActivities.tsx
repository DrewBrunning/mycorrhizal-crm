import ExpandLessIcon from '@mui/icons-material/ExpandLess';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import { Alert, Box, Card, CardContent, Chip, Collapse, Stack, Typography } from '@mui/material';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { SearchResult } from '../api/search';
import { useDateFormat } from '../DateFormatProvider';

interface SearchNotesActivitiesProps {
  query: string;
  result: SearchResult | null;
  onOpenContact?: (id: number) => void;
}

// The cross-entity half of the merged Contacts search (T86): renders the
// notes/activities hits from GET /search in a collapsible section *below* the
// contact cards. The /search `contacts` group is deliberately discarded here —
// the contacts list endpoint is the authority for contacts (it is the one
// that owns the filters, the sort and the cursor). Two disjoint contact lists
// on one page would disagree.
export default function SearchNotesActivities({
  query,
  result,
  onOpenContact,
}: SearchNotesActivitiesProps) {
  const { t } = useTranslation();
  const { formatDate } = useDateFormat();
  const [expanded, setExpanded] = useState(false);

  // A new query starts the section collapsed again — a stale expanded panel
  // left open across a query change would show the old query's hits.
  useEffect(() => {
    setExpanded(false);
  }, [query]);

  if (!result) return null;

  const notes = result.notes || [];
  const activities = result.activities || [];
  const total = notes.length + activities.length;
  const resolvedRelation = result.resolved_relation;

  if (!resolvedRelation && total === 0) return null;

  const toggle = () => setExpanded((v) => !v);

  return (
    <Box sx={{ mt: 2 }}>
      {resolvedRelation && (
        <Alert severity="info" sx={{ py: 0, mb: 1 }}>
          {t('contacts.searchResolvedRelation', {
            relation: t(`relationships.types.${resolvedRelation}`, resolvedRelation),
          })}
        </Alert>
      )}
      {total > 0 && (
        <Card>
          <Box
            onClick={toggle}
            role="button"
            tabIndex={0}
            aria-expanded={expanded}
            onKeyDown={(e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                toggle();
              }
            }}
            sx={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              gap: 1,
              px: 2,
              py: 1.5,
              cursor: 'pointer',
            }}
          >
            <Box>
              <Typography variant="subtitle2" component="h2">
                {t('contacts.searchNotesHeader', { count: total })}
              </Typography>
              <Typography variant="caption" color="text.secondary">
                {t('contacts.searchNotesHint')}
              </Typography>
            </Box>
            {expanded ? <ExpandLessIcon /> : <ExpandMoreIcon />}
          </Box>
          <Collapse in={expanded}>
            <CardContent sx={{ pt: 0, '&:last-child': { pb: 2 } }}>
              {notes.length > 0 && (
                <>
                  <Typography
                    variant="subtitle2"
                    component="h3"
                    color="text.secondary"
                    sx={{ mb: 0.5 }}
                  >
                    {t('contacts.searchNotesGroup')}
                  </Typography>
                  <Stack spacing={1} sx={{ mb: 1.5 }}>
                    {notes.map((n) => (
                      <Box key={n.id}>
                        <Typography variant="body2" sx={{ overflowWrap: 'anywhere' }}>
                          {n.content}
                        </Typography>
                        <Box
                          sx={{
                            display: 'flex',
                            gap: 1,
                            alignItems: 'center',
                            flexWrap: 'wrap',
                            mt: 0.5,
                          }}
                        >
                          {n.contact_name ? (
                            <Chip
                              size="small"
                              label={n.contact_name}
                              onClick={() => n.contact_id && onOpenContact?.(n.contact_id)}
                            />
                          ) : (
                            <Chip size="small" label={t('contacts.searchUnfiled')} />
                          )}
                          <Typography variant="caption" color="text.secondary">
                            {formatDate(n.date)}
                          </Typography>
                        </Box>
                      </Box>
                    ))}
                  </Stack>
                </>
              )}
              {activities.length > 0 && (
                <>
                  <Typography
                    variant="subtitle2"
                    component="h3"
                    color="text.secondary"
                    sx={{ mb: 0.5 }}
                  >
                    {t('contacts.searchActivitiesGroup')}
                  </Typography>
                  <Stack spacing={0.5}>
                    {activities.map((a) => (
                      <Box key={a.id}>
                        <Typography variant="body1">{a.title}</Typography>
                        <Typography variant="caption" color="text.secondary">
                          {formatDate(a.date)}
                        </Typography>
                      </Box>
                    ))}
                  </Stack>
                </>
              )}
            </CardContent>
          </Collapse>
        </Card>
      )}
    </Box>
  );
}
