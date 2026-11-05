import MoreVertIcon from '@mui/icons-material/MoreVert';
import PhotoLibraryIcon from '@mui/icons-material/PhotoLibrary';
import StarIcon from '@mui/icons-material/Star';
import {
  Box,
  Button,
  Chip,
  CircularProgress,
  Dialog,
  DialogContent,
  IconButton,
  Menu,
  MenuItem,
  Typography,
} from '@mui/material';
import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { isPreconditionFailed } from '../api/client';
import {
  adoptContactImage,
  type ContactPhoto,
  clearContactMainImage,
  contactImageSrc,
  deleteContactImage,
  getContactPhotos,
  type ImageCrop,
  type ImageSensitivity,
  patchContactImage,
  setContactMainImage,
  uploadContactImage,
} from '../api/contactImages';
import ImageCropDialog from './ImageCropDialog';

interface ContactImageGalleryProps {
  contactId: number;
  // Read-only in the timeline; the contact's edit surface shows the actions.
  editable?: boolean;
  // Called after a successful add/delete so a parent can refetch a summary.
  onChanged?: () => void;
  // The contact's chosen main image (ContactRecordResponse.main_image_id).
  mainImageId?: number | null;
  // The contact's current revision, sent as If-Match on main-image writes.
  revision?: number;
  // Called after a write that touches the contact itself (main image set or
  // cleared, crop, delete of the main image bump the revision) and after a 412,
  // so the parent refetches the record and passes a fresh revision/main id.
  onContactChanged?: () => void;
}

// Literal keys so the i18n key-usage scan sees them.
const SENSITIVITY_MENU: { value: ImageSensitivity; label: string }[] = [
  { value: 'normal', label: 'imageSensitivity.markNormal' },
  { value: 'private', label: 'imageSensitivity.markPrivate' },
  { value: 'secret', label: 'imageSensitivity.markSecret' },
];

const photoKey = (photo: ContactPhoto) => `${photo.source.kind}-${photo.id}`;

// ContactImageGallery is the contact's Photos tab: the aggregated gallery (own,
// note and activity images, newest first, labelled by source) from
// GET /contacts/{id}/photos. The server returns ready-made relative URLs, so
// the client never guesses a file path (ADR 0040 decision 8). Image ids are only
// unique per owner table, so identity is always (source.kind, id).
//
// Write actions follow the owner rules: only the contact's own images can be set
// as main, cropped or deleted; a note/activity image can be adopted into the
// contact gallery (it is edited on its own note/activity).
export default function ContactImageGallery({
  contactId,
  editable = false,
  onChanged,
  mainImageId,
  revision,
  onContactChanged,
}: ContactImageGalleryProps) {
  const { t } = useTranslation();
  const [photos, setPhotos] = useState<ContactPhoto[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [viewing, setViewing] = useState<ContactPhoto | null>(null);
  const [cropping, setCropping] = useState<ContactPhoto | null>(null);
  const [menu, setMenu] = useState<{ photo: ContactPhoto; anchor: HTMLElement } | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const response = await getContactPhotos(contactId);
      setPhotos(response.photos || []);
      setError('');
    } catch (err) {
      console.error('Failed to load contact photos:', err);
      setError(t('contactImages.loadFailed'));
    } finally {
      setLoading(false);
    }
  }, [contactId, t]);

  useEffect(() => {
    void load();
  }, [load]);

  // Runs one write: busy flag, feedback reset, and a single error path.
  const run = async (action: () => Promise<void>, failure: string) => {
    setBusy(true);
    setError('');
    setNotice('');
    try {
      await action();
    } catch (err) {
      console.error(failure, err);
    } finally {
      setBusy(false);
    }
  };

  const handleFiles = async (files: FileList | null) => {
    if (!files || files.length === 0) return;
    setBusy(true);
    setError('');
    setNotice('');
    try {
      let deduplicated = false;
      for (const file of Array.from(files)) {
        const result = await uploadContactImage(contactId, file);
        deduplicated ||= result.deduplicated;
      }
      await load();
      onChanged?.();
      if (deduplicated) setNotice(t('contactImages.uploadDuplicate'));
    } catch (err) {
      console.error('Failed to upload contact image:', err);
      setError(t('contactImages.uploadFailed'));
    } finally {
      setBusy(false);
      if (inputRef.current) inputRef.current.value = '';
    }
  };

  const handleDelete = (photo: ContactPhoto) =>
    run(async () => {
      try {
        await deleteContactImage(contactId, photo.id);
        setPhotos((prev) => prev.filter((item) => photoKey(item) !== photoKey(photo)));
        onChanged?.();
        // Deleting the main image clears the pointer and bumps the revision.
        onContactChanged?.();
      } catch (err) {
        setError(t('contactImages.deleteFailed'));
        throw err;
      }
    }, 'Failed to delete contact image:');

  const handleSetMain = (photo: ContactPhoto) =>
    run(async () => {
      try {
        await setContactMainImage(contactId, photo.id, revision);
        onContactChanged?.();
      } catch (err) {
        if (isPreconditionFailed(err)) {
          // Another write moved the contact on; refetch so the next try carries
          // the current revision. Nothing was changed.
          setError(t('contactImages.mainStale'));
          onContactChanged?.();
        } else {
          setError(t('contactImages.mainFailed'));
        }
        throw err;
      }
    }, 'Failed to set main image:');

  const handleClearMain = () =>
    run(async () => {
      try {
        await clearContactMainImage(contactId, revision);
        onContactChanged?.();
      } catch (err) {
        if (isPreconditionFailed(err)) {
          setError(t('contactImages.mainStale'));
          onContactChanged?.();
        } else {
          setError(t('contactImages.mainFailed'));
        }
        throw err;
      }
    }, 'Failed to clear main image:');

  const handleCrop = (photo: ContactPhoto, crop: ImageCrop | null) =>
    run(async () => {
      try {
        const updated = await patchContactImage(contactId, photo.id, { crop });
        setPhotos((prev) =>
          prev.map((item) =>
            photoKey(item) === photoKey(photo) ? { ...item, crop: updated.crop ?? crop } : item,
          ),
        );
        setCropping(null);
        // Cropping the main image regenerates the avatar and bumps the revision.
        onContactChanged?.();
      } catch (err) {
        setError(t('contactImages.cropFailed'));
        throw err;
      }
    }, 'Failed to save crop:');

  const handleSensitivity = (photo: ContactPhoto, sensitivity: ImageSensitivity) =>
    run(async () => {
      try {
        const updated = await patchContactImage(contactId, photo.id, { sensitivity });
        setPhotos((prev) =>
          prev.map((item) =>
            photoKey(item) === photoKey(photo)
              ? { ...item, sensitivity: updated.sensitivity ?? sensitivity }
              : item,
          ),
        );
        // A private or secret main image stops being the avatar server-side,
        // which bumps the contact revision.
        onContactChanged?.();
      } catch (err) {
        setError(t('imageSensitivity.failed'));
        throw err;
      }
    }, 'Failed to change image sensitivity:');

  const handleAdopt = (photo: ContactPhoto) =>
    run(async () => {
      try {
        if (photo.source.kind === 'contact') return;
        const result = await adoptContactImage(contactId, {
          source_kind: photo.source.kind,
          source_id: photo.source.id,
          image_id: photo.id,
        });
        await load();
        onChanged?.();
        setNotice(
          result.deduplicated ? t('contactImages.adoptDuplicate') : t('contactImages.adopted'),
        );
      } catch (err) {
        setError(t('contactImages.adoptFailed'));
        throw err;
      }
    }, 'Failed to adopt image:');

  if (loading) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', py: 1 }}>
        <CircularProgress size={20} aria-label={t('contactImages.loading')} />
      </Box>
    );
  }

  if (!editable && photos.length === 0 && !error) return null;

  const isMain = (photo: ContactPhoto) =>
    photo.source.kind === 'contact' && mainImageId != null && photo.id === mainImageId;

  const closeMenu = () => setMenu(null);
  // Closes the menu, then runs the action.
  const choose = (action: () => unknown) => () => {
    closeMenu();
    void action();
  };

  const sourceCaption = (photo: ContactPhoto): string | null => {
    const kind =
      photo.source.kind === 'note'
        ? t('contactImages.sourceNote')
        : photo.source.kind === 'activity'
          ? t('contactImages.sourceActivity')
          : null;
    if (!kind) return null;
    const label = photo.source.label.trim();
    return label ? t('contactImages.sourceLabeled', { kind, label }) : kind;
  };

  return (
    <Box sx={{ mt: 1.5 }}>
      {photos.length > 0 && (
        <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 1 }}>
          {photos.map((photo) => {
            const caption = sourceCaption(photo);
            return (
              <Box
                key={photoKey(photo)}
                data-testid={`contact-photo-${photoKey(photo)}`}
                sx={{ position: 'relative', width: 88 }}
              >
                <Box
                  component="img"
                  src={contactImageSrc(photo.thumbnail_url)}
                  alt={t('contactImages.alt')}
                  loading="lazy"
                  onClick={() => setViewing(photo)}
                  sx={{
                    width: 88,
                    height: 88,
                    objectFit: 'cover',
                    borderRadius: 1,
                    cursor: 'zoom-in',
                    border: 1,
                    borderColor: 'divider',
                    display: 'block',
                  }}
                />
                {isMain(photo) && (
                  <StarIcon
                    fontSize="small"
                    color="primary"
                    aria-label={t('contactImages.mainBadge')}
                    role="img"
                    sx={{ position: 'absolute', top: 2, left: 2 }}
                  />
                )}
                {editable && (
                  <IconButton
                    size="small"
                    aria-label={t('contactImages.actions')}
                    disabled={busy}
                    onClick={(event) => setMenu({ photo, anchor: event.currentTarget })}
                    sx={{ position: 'absolute', top: -8, right: -8, bgcolor: 'background.paper' }}
                  >
                    <MoreVertIcon fontSize="small" />
                  </IconButton>
                )}
                {(caption || photo.sensitivity !== 'normal') && (
                  <Box sx={{ mt: 0.25 }}>
                    {caption && (
                      <Typography
                        variant="caption"
                        color="text.secondary"
                        sx={{ display: 'block' }}
                        noWrap
                        title={caption}
                      >
                        {caption}
                      </Typography>
                    )}
                    {photo.sensitivity !== 'normal' && (
                      <Chip
                        size="small"
                        variant="outlined"
                        color={photo.sensitivity === 'secret' ? 'error' : 'warning'}
                        label={
                          photo.sensitivity === 'secret'
                            ? t('contactImages.sensitivitySecret')
                            : t('contactImages.sensitivityPrivate')
                        }
                      />
                    )}
                  </Box>
                )}
              </Box>
            );
          })}
        </Box>
      )}

      <Menu open={menu !== null} anchorEl={menu?.anchor} onClose={closeMenu}>
        {menu &&
          menu.photo.source.kind === 'contact' && [
            isMain(menu.photo) ? (
              <MenuItem key="clear-main" onClick={choose(handleClearMain)}>
                {t('contactImages.clearMain')}
              </MenuItem>
            ) : (
              // Only a normal-sensitivity image can be the main image (the avatar
              // leaves the instance via CardDAV), so the server rejects the rest.
              menu.photo.sensitivity === 'normal' && (
                <MenuItem key="set-main" onClick={choose(() => handleSetMain(menu.photo))}>
                  {t('contactImages.setMain')}
                </MenuItem>
              )
            ),
            <MenuItem key="crop" onClick={choose(() => setCropping(menu.photo))}>
              {t('contactImages.crop')}
            </MenuItem>,
            ...SENSITIVITY_MENU.filter(({ value }) => value !== menu.photo.sensitivity).map(
              ({ value, label }) => (
                <MenuItem
                  key={`sensitivity-${value}`}
                  onClick={choose(() => handleSensitivity(menu.photo, value))}
                >
                  {t(label)}
                </MenuItem>
              ),
            ),
            <MenuItem key="delete" onClick={choose(() => handleDelete(menu.photo))}>
              {t('contactImages.delete')}
            </MenuItem>,
          ]}
        {menu && menu.photo.source.kind !== 'contact' && (
          <MenuItem onClick={choose(() => handleAdopt(menu.photo))}>
            {t('contactImages.adopt')}
          </MenuItem>
        )}
      </Menu>

      {editable && (
        <Box sx={{ mt: photos.length > 0 ? 1 : 0 }}>
          <input
            ref={inputRef}
            type="file"
            aria-label={t('contactImages.add')}
            accept="image/jpeg,image/png,image/gif"
            multiple
            hidden
            onChange={(event) => void handleFiles(event.target.files)}
          />
          <Button
            size="small"
            startIcon={busy ? <CircularProgress size={16} /> : <PhotoLibraryIcon />}
            disabled={busy}
            onClick={() => inputRef.current?.click()}
          >
            {busy ? t('contactImages.uploading') : t('contactImages.add')}
          </Button>
        </Box>
      )}

      {error && (
        <Typography variant="caption" color="error" role="alert" sx={{ display: 'block', mt: 0.5 }}>
          {error}
        </Typography>
      )}
      {notice && (
        <Typography
          variant="caption"
          color="text.secondary"
          role="status"
          sx={{ display: 'block', mt: 0.5 }}
        >
          {notice}
        </Typography>
      )}

      {cropping && (
        <ImageCropDialog
          photo={cropping}
          busy={busy}
          onSave={(crop) => void handleCrop(cropping, crop)}
          onClear={() => void handleCrop(cropping, null)}
          onClose={() => setCropping(null)}
        />
      )}

      <Dialog open={viewing !== null} onClose={() => setViewing(null)} maxWidth="lg">
        <DialogContent sx={{ p: 0, bgcolor: 'common.black' }}>
          {viewing && (
            <Box
              component="img"
              src={contactImageSrc(viewing.file_url)}
              alt={t('contactImages.alt')}
              sx={{ maxWidth: '90vw', maxHeight: '90vh', display: 'block' }}
            />
          )}
        </DialogContent>
      </Dialog>
    </Box>
  );
}
