import { useTranslation } from 'react-i18next';
import {
  type ActivityImage,
  activityImageSrc,
  deleteActivityImage,
  getActivityImages,
  uploadActivityImage,
} from '../api/activities';
import ImageGallery from './ImageGallery';

interface ActivityImageGalleryProps {
  activityId: number;
  // Read-only in the timeline; an activity's edit surface shows add/remove.
  editable?: boolean;
  // Called after a successful add/delete so a parent can refetch a summary.
  onChanged?: () => void;
}

// ActivityImageGallery renders an activity's images from the dedicated owner-nested
// endpoint. The server already returns ready-made relative URLs, so the client
// never guesses a file path (ADR 0040 decision 8). The behaviour lives in the
// shared ImageGallery (#2312).
export default function ActivityImageGallery({
  activityId,
  editable = false,
  onChanged,
}: ActivityImageGalleryProps) {
  const { t } = useTranslation();
  return (
    <ImageGallery<ActivityImage>
      ownerId={activityId}
      logName="activity"
      labels={{
        loading: t('activityImages.loading'),
        alt: t('activityImages.alt'),
        add: t('activityImages.add'),
        uploading: t('activityImages.uploading'),
        delete: t('activityImages.delete'),
        loadFailed: t('activityImages.loadFailed'),
        uploadFailed: t('activityImages.uploadFailed'),
        deleteFailed: t('activityImages.deleteFailed'),
      }}
      list={async () => (await getActivityImages(activityId)).images || []}
      upload={(file) => uploadActivityImage(activityId, file)}
      remove={(image) => deleteActivityImage(activityId, image.id)}
      imageKey={(image) => String(image.id)}
      thumbnailSrc={(image) => activityImageSrc(image.thumbnail_url)}
      fullSrc={(image) => activityImageSrc(image.file_url)}
      editable={editable}
      onChanged={onChanged}
    />
  );
}
