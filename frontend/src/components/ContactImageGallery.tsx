import MoreVertIcon from '@mui/icons-material/MoreVert';
import StarIcon from '@mui/icons-material/Star';
import { Box, Chip, IconButton, Menu, MenuItem, Typography } from '@mui/material';
import { useState } from 'react';
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
  patchContactImage,
  setContactMainImage,
  uploadContactImage,
} from '../api/contactImages';
import ImageCropDialog from './ImageCropDialog';
import ImageGallery, { type ImageGalleryContext } from './ImageGallery';

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
  const [cropping, setCropping] = useState<ContactPhoto | null>(null);
  const [menu, setMenu] = useState<{ photo: ContactPhoto; anchor: HTMLElement } | null>(null);

  type Ctx = ImageGalleryContext<ContactPhoto>;

  const handleDelete = (ctx: Ctx, photo: ContactPhoto) =>
    ctx.run(async () => {
      try {
        await deleteContactImage(contactId, photo.id);
        ctx.setItems((prev) => prev.filter((item) => photoKey(item) !== photoKey(photo)));
        onChanged?.();
        // Deleting the main image clears the pointer and bumps the revision.
        onContactChanged?.();
      } catch (err) {
        ctx.setError(t('contactImages.deleteFailed'));
        throw err;
      }
    }, 'Failed to delete contact image:');

  // A 412 means another write moved the contact on: refetch so the next try
  // carries the current revision. Nothing was changed.
  const mainFailure = (ctx: Ctx, err: unknown) => {
    if (isPreconditionFailed(err)) {
      ctx.setError(t('contactImages.mainStale'));
      onContactChanged?.();
    } else {
      ctx.setError(t('contactImages.mainFailed'));
    }
  };

  const handleSetMain = (ctx: Ctx, photo: ContactPhoto) =>
    ctx.run(async () => {
      try {
        await setContactMainImage(contactId, photo.id, revision);
        onContactChanged?.();
      } catch (err) {
        mainFailure(ctx, err);
        throw err;
      }
    }, 'Failed to set main image:');

  const handleClearMain = (ctx: Ctx) =>
    ctx.run(async () => {
      try {
        await clearContactMainImage(contactId, revision);
        onContactChanged?.();
      } catch (err) {
        mainFailure(ctx, err);
        throw err;
      }
    }, 'Failed to clear main image:');

  const handleCrop = (ctx: Ctx, photo: ContactPhoto, crop: ImageCrop | null) =>
    ctx.run(async () => {
      try {
        const updated = await patchContactImage(contactId, photo.id, { crop });
        ctx.setItems((prev) =>
          prev.map((item) =>
            photoKey(item) === photoKey(photo) ? { ...item, crop: updated.crop ?? crop } : item,
          ),
        );
        setCropping(null);
        // Cropping the main image regenerates the avatar and bumps the revision.
        onContactChanged?.();
      } catch (err) {
        ctx.setError(t('contactImages.cropFailed'));
        throw err;
      }
    }, 'Failed to save crop:');

  const handleAdopt = (ctx: Ctx, photo: ContactPhoto) =>
    ctx.run(async () => {
      try {
        if (photo.source.kind === 'contact') return;
        const result = await adoptContactImage(contactId, {
          source_kind: photo.source.kind,
          source_id: photo.source.id,
          image_id: photo.id,
        });
        await ctx.reload();
        onChanged?.();
        ctx.setNotice(
          result.deduplicated ? t('contactImages.adoptDuplicate') : t('contactImages.adopted'),
        );
      } catch (err) {
        ctx.setError(t('contactImages.adoptFailed'));
        throw err;
      }
    }, 'Failed to adopt image:');

  const isMain = (photo: ContactPhoto) =>
    photo.source.kind === 'contact' && mainImageId != null && photo.id === mainImageId;

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

  const renderFooter = (photo: ContactPhoto) => {
    const caption = sourceCaption(photo);
    if (!caption && photo.sensitivity === 'normal') return null;
    return (
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
    );
  };

  const renderExtras = (ctx: Ctx) => {
    const closeMenu = () => setMenu(null);
    // Closes the menu, then runs the action.
    const choose = (action: () => unknown) => () => {
      closeMenu();
      void action();
    };
    return (
      <>
        <Menu open={menu !== null} anchorEl={menu?.anchor} onClose={closeMenu}>
          {menu &&
            menu.photo.source.kind === 'contact' && [
              isMain(menu.photo) ? (
                <MenuItem key="clear-main" onClick={choose(() => handleClearMain(ctx))}>
                  {t('contactImages.clearMain')}
                </MenuItem>
              ) : (
                // Only a normal-sensitivity image can be the main image (the avatar
                // leaves the instance via CardDAV), so the server rejects the rest.
                menu.photo.sensitivity === 'normal' && (
                  <MenuItem key="set-main" onClick={choose(() => handleSetMain(ctx, menu.photo))}>
                    {t('contactImages.setMain')}
                  </MenuItem>
                )
              ),
              <MenuItem key="crop" onClick={choose(() => setCropping(menu.photo))}>
                {t('contactImages.crop')}
              </MenuItem>,
              <MenuItem key="delete" onClick={choose(() => handleDelete(ctx, menu.photo))}>
                {t('contactImages.delete')}
              </MenuItem>,
            ]}
          {menu && menu.photo.source.kind !== 'contact' && (
            <MenuItem onClick={choose(() => handleAdopt(ctx, menu.photo))}>
              {t('contactImages.adopt')}
            </MenuItem>
          )}
        </Menu>

        {cropping && (
          <ImageCropDialog
            photo={cropping}
            busy={ctx.busy}
            onSave={(crop) => void handleCrop(ctx, cropping, crop)}
            onClear={() => void handleCrop(ctx, cropping, null)}
            onClose={() => setCropping(null)}
          />
        )}
      </>
    );
  };

  return (
    <ImageGallery<ContactPhoto>
      ownerId={contactId}
      logName="contact"
      labels={{
        loading: t('contactImages.loading'),
        alt: t('contactImages.alt'),
        add: t('contactImages.add'),
        uploading: t('contactImages.uploading'),
        delete: t('contactImages.delete'),
        loadFailed: t('contactImages.loadFailed'),
        uploadFailed: t('contactImages.uploadFailed'),
        deleteFailed: t('contactImages.deleteFailed'),
      }}
      list={async () => (await getContactPhotos(contactId)).photos || []}
      upload={(file) => uploadContactImage(contactId, file)}
      imageKey={photoKey}
      thumbnailSrc={(photo) => contactImageSrc(photo.thumbnail_url)}
      fullSrc={(photo) => contactImageSrc(photo.file_url)}
      itemTestId={(photo) => `contact-photo-${photoKey(photo)}`}
      editable={editable}
      onChanged={onChanged}
      renderOverlay={(photo) =>
        isMain(photo) && (
          <StarIcon
            fontSize="small"
            color="primary"
            aria-label={t('contactImages.mainBadge')}
            role="img"
            sx={{ position: 'absolute', top: 2, left: 2 }}
          />
        )
      }
      renderActions={(photo, ctx) => (
        <IconButton
          size="small"
          aria-label={t('contactImages.actions')}
          disabled={ctx.busy}
          onClick={(event) => setMenu({ photo, anchor: event.currentTarget })}
          sx={{ position: 'absolute', top: -8, right: -8, bgcolor: 'background.paper' }}
        >
          <MoreVertIcon fontSize="small" />
        </IconButton>
      )}
      renderFooter={renderFooter}
      renderExtras={renderExtras}
    />
  );
}
