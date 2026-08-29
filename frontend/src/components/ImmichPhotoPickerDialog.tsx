import {
  Alert,
  Box,
  Button,
  CircularProgress,
  DialogActions,
  DialogContent,
  DialogTitle,
  Typography,
} from '@mui/material';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { apiFetch } from '../api/client';
import {
  getImmichContactAssets,
  type ImmichAssetSummary,
  type ImmichPerson,
  immichAssetImageUrl,
  immichThumbnailUrl,
} from '../api/immich';
import { getErrorMessage } from '../utils/errorHandler';
import AppDialog from './AppDialog';
import AuthImg from './AuthImg';
import ImmichPersonSearchDialog from './ImmichPersonSearchDialog';

interface ImmichPhotoPickerDialogProps {
  open: boolean;
  onClose: () => void;
  contactUid: string;
  // Whether this contact already has a linked Immich person.
  isLinked: boolean;
  onFetchPeople: () => Promise<ImmichPerson[]>;
  onLinkPerson: (person: ImmichPerson) => Promise<void>;
  // Called with a data URL once a photo is picked — feeds straight into
  // ProfilePictureUploadDialog's existing crop step, unchanged.
  onImageSelected: (dataUrl: string) => void;
}

// ImmichPhotoPickerDialog lets a user set a contact's profile photo from
// their Immich library — "choose from Immich" alongside the existing
// upload/URL options. If the contact isn't linked to an Immich person yet,
// this delegates entirely to ImmichPersonSearchDialog first (linking happens
// right here, no separate trip to the External Links panel), then switches
// to browsing that (now-linked) person's recent photos. Only one dialog is
// ever mounted at a time — never both — so this never nests two modals.
export default function ImmichPhotoPickerDialog({
  open,
  onClose,
  contactUid,
  isLinked,
  onFetchPeople,
  onLinkPerson,
  onImageSelected,
}: ImmichPhotoPickerDialogProps) {
  const { t } = useTranslation();
  const [linked, setLinked] = useState(isLinked);
  const [assets, setAssets] = useState<ImmichAssetSummary[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [selecting, setSelecting] = useState(false);

  useEffect(() => {
    if (!open) return;
    setLinked(isLinked);
    setAssets([]);
    setError('');
    setSelecting(false);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  useEffect(() => {
    if (!open || !linked) return;
    setLoading(true);
    setError('');
    getImmichContactAssets(contactUid)
      .then((fetched) => setAssets(fetched || []))
      .catch((err) => setError(getErrorMessage(err)))
      .finally(() => setLoading(false));
  }, [open, linked, contactUid]);

  // Linking happens through the delegated ImmichPersonSearchDialog below —
  // a failure there is handled by that dialog's own error display, so a
  // rejection here just leaves `linked` false and lets it propagate.
  const handleLink = async (person: ImmichPerson) => {
    await onLinkPerson(person);
    setLinked(true);
  };

  const handlePick = async (url: string) => {
    setSelecting(true);
    setError('');
    try {
      const response = await apiFetch(url);
      if (!response.ok) throw new Error(t('immich.photoPicker.loadFailed'));
      const blob = await response.blob();
      const reader = new FileReader();
      reader.addEventListener('load', () => {
        onImageSelected(reader.result as string);
      });
      reader.readAsDataURL(blob);
    } catch (err) {
      setError(getErrorMessage(err));
    } finally {
      setSelecting(false);
    }
  };

  if (!linked) {
    return (
      <ImmichPersonSearchDialog
        open={open}
        onClose={onClose}
        onFetchPeople={onFetchPeople}
        onSelect={handleLink}
      />
    );
  }

  return (
    <AppDialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>{t('immich.photoPicker.title')}</DialogTitle>
      <DialogContent>
        {loading && <CircularProgress size={24} />}
        {!loading && error && (
          <Alert severity="error" sx={{ mb: 1.5 }}>
            {error}
          </Alert>
        )}
        {!loading && !error && assets.length === 0 && (
          <Typography
            variant="body2"
            sx={{
              color: 'text.secondary',
              py: 2,
              textAlign: 'center',
            }}
          >
            {t('immich.photoPicker.noPhotos')}
          </Typography>
        )}
        {!loading && (
          <Box
            sx={{ display: 'flex', flexWrap: 'wrap', gap: 1, maxHeight: 360, overflowY: 'auto' }}
          >
            <Box
              component="button"
              type="button"
              onClick={() => handlePick(immichThumbnailUrl(contactUid))}
              disabled={selecting}
              sx={{
                p: 0,
                border: 'none',
                cursor: 'pointer',
                borderRadius: 1,
                overflow: 'hidden',
                opacity: selecting ? 0.5 : 1,
                lineHeight: 0,
              }}
            >
              <AuthImg
                src={immichThumbnailUrl(contactUid)}
                alt={t('immich.photoPicker.primaryPhotoAlt')}
                sx={{ width: 100, height: 100, objectFit: 'cover', bgcolor: 'action.hover' }}
              />
            </Box>
            {assets.map((asset) => (
              <Box
                key={asset.id}
                component="button"
                type="button"
                onClick={() => handlePick(immichAssetImageUrl(contactUid, asset.id))}
                disabled={selecting}
                sx={{
                  p: 0,
                  border: 'none',
                  cursor: 'pointer',
                  borderRadius: 1,
                  overflow: 'hidden',
                  opacity: selecting ? 0.5 : 1,
                  lineHeight: 0,
                }}
              >
                <AuthImg
                  src={immichAssetImageUrl(contactUid, asset.id)}
                  alt={t('immich.photoPicker.photoAlt')}
                  sx={{ width: 100, height: 100, objectFit: 'cover', bgcolor: 'action.hover' }}
                />
              </Box>
            ))}
          </Box>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={selecting}>
          {t('common.cancel')}
        </Button>
      </DialogActions>
    </AppDialog>
  );
}
