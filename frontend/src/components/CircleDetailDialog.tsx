import {
  Box,
  Button,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  List,
  ListItem,
  ListItemText,
  Stack,
  TextField,
  ToggleButton,
  ToggleButtonGroup,
  Typography,
} from '@mui/material';
import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  CIRCLE_COLORS,
  CIRCLE_DESCRIPTION_MAX,
  type Circle,
  type CircleColor,
  type CircleMetadata,
  type CircleTimelineResponse,
  getCircleTimeline,
} from '../api/circles';
import type { TimelineItem } from '../api/timeline';
import { circleColorHex } from '../utils/circleColors';
import { handleFetchError } from '../utils/errorHandler';

// Issue #1995: edit a circle's description/colour and see its members' merged
// timeline ("aggregate" Part 2 decision: no circle-owned notes/activities).
// The caller keys this component by circle id, so form state initializes per circle.
interface CircleDetailDialogProps {
  circle: Circle | null;
  memberCount: number;
  onClose: () => void;
  onSave: (id: string, name: string, metadata: CircleMetadata) => Promise<void>;
}

// A short human summary of a timeline entry: the first non-empty text field.
function summarize(item: TimelineItem): string {
  const data = item.data as unknown as Record<string, unknown>;
  for (const key of ['content', 'title', 'message', 'name', 'description', 'type']) {
    const v = data[key];
    if (typeof v === 'string' && v.trim()) return v;
  }
  return '';
}

export default function CircleDetailDialog({
  circle,
  memberCount,
  onClose,
  onSave,
}: CircleDetailDialogProps) {
  const { t } = useTranslation();
  const [name, setName] = useState(circle?.name ?? '');
  const [description, setDescription] = useState(circle?.description ?? '');
  const [color, setColor] = useState<CircleColor>(circle?.color ?? '');
  const [saving, setSaving] = useState(false);
  const [page, setPage] = useState<CircleTimelineResponse | null>(null);
  const [items, setItems] = useState<TimelineItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const circleId = circle?.id;
  const load = useCallback(async (id: string, cursor?: string) => {
    setLoading(true);
    setError(null);
    try {
      const res = await getCircleTimeline(id, { cursor });
      setPage(res);
      setItems((prev) => (cursor ? [...prev, ...res.items] : res.items));
    } catch (err) {
      setError(handleFetchError(err, 'fetching circle timeline'));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (!circleId) return;
    setItems([]);
    setPage(null);
    void load(circleId);
  }, [circleId, load]);

  if (!circle) return null;

  const trimmedName = name.trim();
  const dirty =
    trimmedName !== circle.name ||
    description.trim() !== (circle.description ?? '') ||
    color !== (circle.color ?? '');

  const handleSave = async () => {
    if (!trimmedName) return;
    setSaving(true);
    try {
      await onSave(circle.id, trimmedName, { description: description.trim(), color });
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open onClose={onClose} fullWidth maxWidth="sm">
      <DialogTitle>{circle.name}</DialogTitle>
      <DialogContent>
        <Stack spacing={2} sx={{ mt: 1 }}>
          <TextField
            label={t('circlesTags.nameLabel')}
            size="small"
            value={name}
            onChange={(e) => setName(e.target.value)}
            fullWidth
          />
          <TextField
            label={t('circlesTags.descriptionLabel')}
            size="small"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            multiline
            minRows={2}
            fullWidth
            slotProps={{ htmlInput: { maxLength: CIRCLE_DESCRIPTION_MAX } }}
            helperText={`${description.length}/${CIRCLE_DESCRIPTION_MAX}`}
          />
          <Box>
            <Typography variant="body2" sx={{ color: 'text.secondary', mb: 0.5 }}>
              {t('circlesTags.colorLabel')}
            </Typography>
            <ToggleButtonGroup
              exclusive
              size="small"
              value={color}
              onChange={(_, v: CircleColor | null) => setColor(v ?? '')}
              aria-label={t('circlesTags.colorLabel')}
              sx={{ flexWrap: 'wrap' }}
            >
              <ToggleButton value="" aria-label={t('circlesTags.colorNone')}>
                {t('circlesTags.colorNone')}
              </ToggleButton>
              {CIRCLE_COLORS.map((c) => (
                <ToggleButton key={c} value={c} aria-label={t(`circlesTags.colors.${c}`)}>
                  <Box
                    sx={{
                      width: 16,
                      height: 16,
                      borderRadius: '50%',
                      bgcolor: circleColorHex(c),
                    }}
                  />
                </ToggleButton>
              ))}
            </ToggleButtonGroup>
          </Box>

          <Box>
            <Typography variant="subtitle1">{t('circlesTags.timelineTitle')}</Typography>
            <Typography variant="body2" sx={{ color: 'text.secondary' }}>
              {t('circlesTags.memberCount', { count: page?.member_count ?? memberCount })}
            </Typography>
            {error && (
              <Typography color="error" role="alert">
                {error}
              </Typography>
            )}
            {loading && items.length === 0 && <CircularProgress size={20} sx={{ mt: 1 }} />}
            {!loading && !error && items.length === 0 && page && (
              <Typography sx={{ color: 'text.secondary', mt: 1 }}>
                {t('circlesTags.timelineEmpty')}
              </Typography>
            )}
            {items.length > 0 && (
              <List dense disablePadding>
                {items.map((item) => (
                  <ListItem key={`${item.type}-${item.id}`} disableGutters divider>
                    <ListItemText
                      primary={summarize(item)}
                      secondary={`${t(`circlesTags.timelineTypes.${item.type}`)} · ${new Date(item.date).toLocaleDateString()}`}
                      slotProps={{ primary: { sx: { overflowWrap: 'anywhere' } } }}
                    />
                  </ListItem>
                ))}
              </List>
            )}
            {page?.next_cursor && (
              <Button
                size="small"
                disabled={loading}
                onClick={() => void load(circle.id, page.next_cursor)}
              >
                {t('circlesTags.timelineLoadMore')}
              </Button>
            )}
          </Box>
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>{t('common.close')}</Button>
        <Button
          variant="contained"
          onClick={() => void handleSave()}
          disabled={!trimmedName || !dirty || saving}
        >
          {t('common.save')}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
