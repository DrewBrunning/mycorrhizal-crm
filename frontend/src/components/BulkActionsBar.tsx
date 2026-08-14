import { useState } from 'react';
import {
  Box,
  Button,
  Checkbox,
  FormControl,
  FormControlLabel,
  InputLabel,
  MenuItem,
  Select,
  Tooltip,
  Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { Circle } from '../api/circles';
import { Tag } from '../api/tags';

interface BulkActionsBarProps {
  selectedCount: number;
  loadedCount: number;
  allSelected: boolean;
  circles: Circle[];
  tags: Tag[];
  busy: boolean;
  onSelectAll: () => void;
  onClear: () => void;
  onAddCircle: (circleId: string) => Promise<void>;
  onRemoveCircle: (circleId: string) => Promise<void>;
  onAddTag: (tagId: string) => Promise<void>;
  onRemoveTag: (tagId: string) => Promise<void>;
  onArchive: () => Promise<void>;
  onUnarchive: () => Promise<void>;
  onMerge: () => void;
  onDelete: () => Promise<void>;
}

// The bulk-action bar for the contacts list (N5). The "Select all" control is
// always visible once there are contacts; the actions appear only while a
// selection is active. Selection is keyed by Contact.VCardUID and survives
// pagination ("load more" appends without touching it).
export default function BulkActionsBar({
  selectedCount,
  loadedCount,
  allSelected,
  circles,
  tags,
  busy,
  onSelectAll,
  onClear,
  onAddCircle,
  onRemoveCircle,
  onAddTag,
  onRemoveTag,
  onArchive,
  onUnarchive,
  onMerge,
  onDelete,
}: BulkActionsBarProps) {
  const { t } = useTranslation();
  const [circleId, setCircleId] = useState('');
  const [tagId, setTagId] = useState('');

  if (loadedCount === 0) return null;

  return (
    <Box
      sx={{
        mb: 2,
        p: 1.5,
        bgcolor: 'action.selected',
        borderRadius: 1,
        display: 'flex',
        alignItems: 'center',
        gap: 1,
        flexWrap: 'wrap',
      }}
    >
      <FormControlLabel
        control={
          <Checkbox
            size="small"
            checked={allSelected && selectedCount > 0}
            indeterminate={selectedCount > 0 && !allSelected}
            disabled={busy || loadedCount === 0}
            onChange={onSelectAll}
            inputProps={{ 'aria-label': t('bulk.selectAll') }}
          />
        }
        label={t('bulk.selectAll')}
        sx={{ mr: 1 }}
      />

      {selectedCount > 0 && (
        <>
          <Typography variant="body2" sx={{ fontWeight: 500, whiteSpace: 'nowrap' }}>
            {t('bulk.selected', { count: selectedCount })}
          </Typography>
          <Button size="small" onClick={onClear} disabled={busy}>
            {t('bulk.clear')}
          </Button>

          <FormControl size="small" sx={{ minWidth: 160 }}>
            <InputLabel id="bulk-circle-label">{t('bulk.selectCircle')}</InputLabel>
            <Select
              labelId="bulk-circle-label"
              label={t('bulk.selectCircle')}
              value={circleId}
              onChange={(e) => setCircleId(e.target.value)}
              disabled={busy}
              size="small"
            >
              {circles.map((circle) => (
                <MenuItem key={circle.id} value={circle.id}>
                  {circle.name}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
          <Button
            size="small"
            variant="outlined"
            disabled={busy || !circleId}
            onClick={() => onAddCircle(circleId)}
          >
            {t('bulk.addCircle')}
          </Button>
          <Button
            size="small"
            variant="outlined"
            disabled={busy || !circleId}
            onClick={() => onRemoveCircle(circleId)}
          >
            {t('bulk.removeCircle')}
          </Button>

          <FormControl size="small" sx={{ minWidth: 140 }}>
            <InputLabel id="bulk-tag-label">{t('bulk.selectTag')}</InputLabel>
            <Select
              labelId="bulk-tag-label"
              label={t('bulk.selectTag')}
              value={tagId}
              onChange={(e) => setTagId(e.target.value)}
              disabled={busy}
              size="small"
            >
              {tags.map((tag) => (
                <MenuItem key={tag.id} value={tag.id}>
                  {tag.name}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
          <Button size="small" variant="outlined" disabled={busy || !tagId} onClick={() => onAddTag(tagId)}>
            {t('bulk.addTag')}
          </Button>
          <Button size="small" variant="outlined" disabled={busy || !tagId} onClick={() => onRemoveTag(tagId)}>
            {t('bulk.removeTag')}
          </Button>

          <Button size="small" variant="outlined" disabled={busy} onClick={onArchive}>
            {t('bulk.archive')}
          </Button>
          <Button size="small" variant="outlined" disabled={busy} onClick={onUnarchive}>
            {t('bulk.unarchive')}
          </Button>
          {/* T92: merge is pairwise, not a one-verb-over-N-rows action, so it
              is only meaningful (and only enabled) for exactly two selected
              contacts. Anything else shows it disabled with a tooltip
              explaining the constraint. */}
          <Tooltip title={selectedCount === 2 ? '' : t('bulk.mergeSelectTwoHint')}>
            <span>
              <Button
                size="small"
                variant="outlined"
                disabled={busy || selectedCount !== 2}
                onClick={onMerge}
              >
                {t('bulk.merge')}
              </Button>
            </span>
          </Tooltip>
          <Button size="small" variant="outlined" color="error" disabled={busy} onClick={onDelete}>
            {t('bulk.delete')}
          </Button>
        </>
      )}
    </Box>
  );
}
