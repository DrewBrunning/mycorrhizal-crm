import {
  Box,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Slider,
  Typography,
} from '@mui/material';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { type ContactPhoto, contactImageSrc, type ImageCrop } from '../api/contactImages';
import { cropToControls, MIN_CROP_ZOOM, squareCrop } from '../utils/imageCrop';

interface ImageCropDialogProps {
  photo: ContactPhoto;
  busy?: boolean;
  onSave: (crop: ImageCrop) => void;
  // Drops the stored crop so the whole photo is used.
  onClear: () => void;
  onClose: () => void;
}

// Square avatar-crop editor for one of the contact's own images (ADR 0040
// decision 10; the same controls as Android's ImageCropDialog). The stored
// image is never cropped destructively; the saved fractions only steer the
// server-derived avatar. Opens at the stored crop when there is one.
export default function ImageCropDialog({
  photo,
  busy = false,
  onSave,
  onClear,
  onClose,
}: ImageCropDialogProps) {
  const { t } = useTranslation();
  const initial = cropToControls(photo.width, photo.height, photo.crop);
  const [zoom, setZoom] = useState(initial.zoom);
  const [posX, setPosX] = useState(initial.posX);
  const [posY, setPosY] = useState(initial.posY);
  const crop = squareCrop(photo.width, photo.height, zoom, posX, posY);
  const aspect = photo.width > 0 && photo.height > 0 ? photo.width / photo.height : 1;
  const pct = (value: number) => `${value * 100}%`;

  const slider = (label: string, value: number, set: (value: number) => void, min = 0) => (
    <Box>
      <Typography variant="caption">{label}</Typography>
      <Slider
        size="small"
        value={value}
        min={min}
        max={1}
        step={0.01}
        aria-label={label}
        onChange={(_, next) => set(Array.isArray(next) ? next[0] : next)}
      />
    </Box>
  );

  return (
    <Dialog open onClose={onClose} fullWidth maxWidth="xs">
      <DialogTitle>{t('contactImages.cropTitle')}</DialogTitle>
      <DialogContent>
        <Box
          data-testid="crop-preview"
          sx={{ position: 'relative', width: '100%', aspectRatio: String(aspect), mb: 1 }}
        >
          <Box
            component="img"
            src={contactImageSrc(photo.thumbnail_url)}
            alt={t('contactImages.alt')}
            sx={{ width: '100%', height: '100%', display: 'block' }}
          />
          {/* The box-shadow dims everything outside the crop square. */}
          <Box
            data-testid="crop-rect"
            sx={{
              position: 'absolute',
              left: pct(crop.x),
              top: pct(crop.y),
              width: pct(crop.w),
              height: pct(crop.h),
              border: 2,
              borderColor: 'primary.main',
              boxShadow: '0 0 0 9999px rgba(0, 0, 0, 0.5)',
              pointerEvents: 'none',
            }}
          />
        </Box>
        {slider(t('contactImages.cropZoom'), zoom, setZoom, MIN_CROP_ZOOM)}
        {slider(t('contactImages.cropHorizontal'), posX, setPosX)}
        {slider(t('contactImages.cropVertical'), posY, setPosY)}
      </DialogContent>
      <DialogActions sx={{ flexWrap: 'wrap' }}>
        <Button onClick={onClear} disabled={busy} sx={{ mr: 'auto' }}>
          {t('contactImages.cropReset')}
        </Button>
        <Button onClick={onClose} disabled={busy}>
          {t('common.cancel')}
        </Button>
        <Button variant="contained" onClick={() => onSave(crop)} disabled={busy}>
          {t('contactImages.cropSave')}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
