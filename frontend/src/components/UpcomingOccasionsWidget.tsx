import CelebrationIcon from '@mui/icons-material/Celebration';
import {
  Box,
  Card,
  CardContent,
  Chip,
  Stack,
  ToggleButton,
  ToggleButtonGroup,
  Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';
import type { UpcomingOccasion } from '../api/occasionObligations';

interface UpcomingOccasionsWidgetProps {
  occasions: UpcomingOccasion[];
  loading: boolean;
  error: string | null;
  days: 30 | 90;
  onDaysChange: (days: 30 | 90) => void;
}

// The dashboard's "upcoming occasions" widget (ADR 0024 part 3, issue #387):
// every birthday/anniversary/life-event/active-obligation occurrence within
// the chosen window, sorted ascending by days-until. Mirrors the existing
// Upcoming Birthdays column's Card-list layout.
export default function UpcomingOccasionsWidget({
  occasions,
  loading,
  error,
  days,
  onDaysChange,
}: UpcomingOccasionsWidgetProps) {
  const { t } = useTranslation();

  return (
    <Box>
      <Box sx={{ mb: 1.5, display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap' }}>
        <CelebrationIcon color="primary" fontSize="small" />
        <Typography variant="subtitle1" component="h2" sx={{ fontWeight: 500, flexGrow: 1 }}>
          {t('occasions.widget.title')}
        </Typography>
        <ToggleButtonGroup
          size="small"
          exclusive
          value={days}
          onChange={(_e, value) => {
            if (value != null) onDaysChange(value);
          }}
          aria-label={t('occasions.widget.windowLabel')}
        >
          {/* MUI's default unselected ToggleButton color (action.active,
              rgba(0,0,0,0.54)) composites to ~4.48:1 against this theme's
              parchment surface (#faf5ea) -- just under the 4.5:1 WCAG AA
              floor (caught by the fixtures.ts per-test axe scan, since the
              dashboard is where most specs end up). text.secondary is this
              theme's own AA-checked muted color (7.17:1) and reads the same
              visual weight. */}
          <ToggleButton value={30} sx={{ color: 'text.secondary' }}>
            {t('occasions.widget.days30')}
          </ToggleButton>
          <ToggleButton value={90} sx={{ color: 'text.secondary' }}>
            {t('occasions.widget.days90')}
          </ToggleButton>
        </ToggleButtonGroup>
      </Box>

      {error && (
        <Typography variant="body2" color="error" sx={{ mb: 1 }}>
          {error}
        </Typography>
      )}

      {!loading && !error && occasions.length === 0 ? (
        <Card>
          <CardContent sx={{ py: 2 }}>
            <Typography variant="body2" sx={{ color: 'text.secondary' }}>
              {t('occasions.widget.empty')}
            </Typography>
          </CardContent>
        </Card>
      ) : (
        <Stack spacing={1.5}>
          {occasions.map((o, index) => (
            <Card
              // biome-ignore lint/suspicious/noArrayIndexKey: contact_id may repeat across sources
              key={`${o.source}-${o.contact_id}-${index}`}
              component={Link}
              to={`/contacts/${o.contact_id}`}
              sx={{
                textDecoration: 'none',
                border: '1px solid',
                borderColor: o.days_until === 0 ? 'success.main' : 'divider',
                '&:hover': { boxShadow: 2, transform: 'translateY(-1px)', transition: 'all 0.2s' },
              }}
            >
              <CardContent sx={{ py: 1.5 }}>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>
                  <Box sx={{ flexGrow: 1 }}>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, flexWrap: 'wrap' }}>
                      <Chip
                        size="small"
                        sx={{ height: 18 }}
                        label={t(`occasions.sources.${o.source}`)}
                      />
                      {o.kind && (
                        <Chip
                          size="small"
                          sx={{ height: 18 }}
                          label={t(`occasions.kinds.${o.kind}`, o.kind)}
                        />
                      )}
                    </Box>
                    <Typography variant="body2" sx={{ fontWeight: 500, mt: 0.25 }}>
                      {o.contact_name}
                      {o.source !== 'birthday' && o.source !== 'anniversary' ? ` — ${o.label}` : ''}
                    </Typography>
                    <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                      {o.days_until === 0
                        ? t('occasions.widget.today')
                        : t('occasions.widget.inDays', { count: o.days_until })}
                    </Typography>
                  </Box>
                </Box>
              </CardContent>
            </Card>
          ))}
        </Stack>
      )}
    </Box>
  );
}
