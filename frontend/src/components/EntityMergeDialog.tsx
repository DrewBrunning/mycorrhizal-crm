import SwapVertIcon from '@mui/icons-material/SwapVert';
import {
  Alert,
  Autocomplete,
  Box,
  Button,
  Checkbox,
  CircularProgress,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  FormGroup,
  IconButton,
  TextField,
  Tooltip,
  Typography,
} from '@mui/material';
import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  type Entity,
  type EntityMergeResponse,
  getEntity,
  listEntities,
  mergeEntity,
} from '../api/entities';
import { fillableMergeFields } from '../utils/entityCore';
import { ENTITY_KIND_KEYS } from '../utils/entityLabels';
import { getErrorMessage } from '../utils/errorHandler';
import AppDialog from './AppDialog';

// Field key (as the backend names it in `resolutions`) -> label key.
const FIELD_LABEL_KEYS: Record<string, string> = {
  'organization.sortAs': 'entities.fields.sortAs',
  'organization.units': 'entities.fields.units',
  url: 'entities.fields.url',
  notes: 'entities.fields.notes',
  address: 'entities.fields.address',
  species: 'entities.fields.species',
  breed: 'entities.fields.breed',
  birth: 'entities.fields.birth',
  death: 'entities.fields.death',
};

interface EntityMergeDialogProps {
  open: boolean;
  /** The entity that is merged away (soft-deleted, restorable). */
  loserId: string;
  /** The entity that is kept. Omit to let the user pick one of the same kind. */
  survivorId?: string;
  onClose: () => void;
  onMerged: (result: EntityMergeResponse, loser: Entity) => void;
}

// Merge dialog (ADR 0043 decision 8): a merge only happens on an explicit
// confirm here, and empty fields are only filled from the other entity where
// the user ticks them (field-by-field, no silent union).
export default function EntityMergeDialog({
  open,
  loserId,
  survivorId,
  onClose,
  onMerged,
}: EntityMergeDialogProps) {
  const { t } = useTranslation();
  const [loser, setLoser] = useState<Entity | null>(null);
  const [survivor, setSurvivor] = useState<Entity | null>(null);
  const [options, setOptions] = useState<Entity[]>([]);
  const [ticked, setTicked] = useState<Set<string>>(new Set());
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!open) return;
    let cancelled = false;
    setLoser(null);
    setSurvivor(null);
    setTicked(new Set());
    setError('');
    setLoading(true);
    void (async () => {
      try {
        const [l, s] = await Promise.all([
          getEntity(loserId),
          survivorId ? getEntity(survivorId) : null,
        ]);
        if (cancelled) return;
        setLoser(l);
        setSurvivor(s);
        if (!survivorId) {
          const page = await listEntities({ kind: l.kind, limit: 100 });
          if (!cancelled) setOptions(page.entities.filter((e) => e.id !== l.id));
        }
      } catch (err) {
        if (!cancelled) setError(getErrorMessage(err));
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [open, loserId, survivorId]);

  const fillable = useMemo(
    () => (loser && survivor ? fillableMergeFields(survivor, loser) : []),
    [loser, survivor],
  );

  const swap = () => {
    setLoser(survivor);
    setSurvivor(loser);
    setTicked(new Set());
  };

  const toggle = (field: string) =>
    setTicked((prev) => {
      const next = new Set(prev);
      if (next.has(field)) next.delete(field);
      else next.add(field);
      return next;
    });

  const handleConfirm = async () => {
    if (!loser || !survivor) return;
    setSaving(true);
    setError('');
    try {
      const resolutions = Object.fromEntries(
        fillable.filter((f) => ticked.has(f)).map((f) => [f, 'loser' as const]),
      );
      const result = await mergeEntity(loser.id, {
        into: survivor.id,
        ...(Object.keys(resolutions).length > 0 ? { resolutions } : {}),
      });
      onMerged(result, loser);
    } catch (err) {
      setError(getErrorMessage(err) || t('entities.merge.failed'));
    } finally {
      setSaving(false);
    }
  };

  return (
    <AppDialog
      open={open}
      onClose={onClose}
      maxWidth="sm"
      fullWidth
      aria-labelledby="entity-merge-title"
    >
      <DialogTitle id="entity-merge-title">
        {t('entities.merge.title', { name: loser?.name ?? '' })}
      </DialogTitle>
      <DialogContent>
        <Box sx={{ pt: 1, display: 'flex', flexDirection: 'column', gap: 2 }}>
          {loading && <CircularProgress size={24} aria-label={t('common.loading')} />}
          {!survivorId && loser && (
            <Autocomplete
              options={options}
              value={survivor}
              onChange={(_, v) => {
                setSurvivor(v);
                setTicked(new Set());
              }}
              getOptionLabel={(o) => o.name}
              isOptionEqualToValue={(a, b) => a.id === b.id}
              noOptionsText={t('entities.merge.noCandidates', {
                kind: t(ENTITY_KIND_KEYS[loser.kind]).toLowerCase(),
              })}
              renderInput={(params) => <TextField {...params} label={t('entities.merge.into')} />}
            />
          )}
          {loser && survivor && (
            <>
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                <Typography sx={{ flex: 1 }}>
                  {t('entities.merge.intro', { loser: loser.name, survivor: survivor.name })}
                </Typography>
                {survivorId && (
                  <Tooltip title={t('entities.merge.swap')}>
                    <IconButton aria-label={t('entities.merge.swap')} onClick={swap}>
                      <SwapVertIcon />
                    </IconButton>
                  </Tooltip>
                )}
              </Box>
              {fillable.length > 0 && (
                <FormGroup>
                  <Typography variant="body2" sx={{ mb: 0.5 }}>
                    {t('entities.merge.fill', { name: loser.name })}
                  </Typography>
                  {fillable.map((f) => (
                    <FormControlLabel
                      key={f}
                      control={<Checkbox checked={ticked.has(f)} onChange={() => toggle(f)} />}
                      label={t(FIELD_LABEL_KEYS[f] ?? 'entities.fields.notes')}
                    />
                  ))}
                </FormGroup>
              )}
            </>
          )}
          {error && <Alert severity="error">{error}</Alert>}
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={saving}>
          {t('common.cancel')}
        </Button>
        <Button
          variant="contained"
          color="warning"
          onClick={() => void handleConfirm()}
          disabled={saving || !loser || !survivor}
        >
          {t('entities.merge.confirm')}
        </Button>
      </DialogActions>
    </AppDialog>
  );
}
