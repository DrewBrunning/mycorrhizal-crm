import { useTranslation } from 'react-i18next';
import {
  deleteNoteImage,
  getNoteImages,
  type NoteImage,
  noteImageSrc,
  uploadNoteImage,
} from '../api/notes';
import ImageGallery from './ImageGallery';

interface NoteImageGalleryProps {
  noteId: number;
  // Read-only in the timeline; a note's edit surface shows add/remove.
  editable?: boolean;
  // Called after a successful add/delete so a parent can refetch a summary.
  onChanged?: () => void;
}

// NoteImageGallery renders a note's images from the dedicated owner-nested
// endpoint. The server already returns ready-made relative URLs, so the client
// never guesses a file path (ADR 0040 decision 8). The behaviour lives in the
// shared ImageGallery (#2312).
export default function NoteImageGallery({
  noteId,
  editable = false,
  onChanged,
}: NoteImageGalleryProps) {
  const { t } = useTranslation();
  return (
    <ImageGallery<NoteImage>
      ownerId={noteId}
      logName="note"
      labels={{
        loading: t('noteImages.loading'),
        alt: t('noteImages.alt'),
        add: t('noteImages.add'),
        uploading: t('noteImages.uploading'),
        delete: t('noteImages.delete'),
        loadFailed: t('noteImages.loadFailed'),
        uploadFailed: t('noteImages.uploadFailed'),
        deleteFailed: t('noteImages.deleteFailed'),
      }}
      list={async () => (await getNoteImages(noteId)).images || []}
      upload={(file) => uploadNoteImage(noteId, file)}
      remove={(image) => deleteNoteImage(noteId, image.id)}
      imageKey={(image) => String(image.id)}
      thumbnailSrc={(image) => noteImageSrc(image.thumbnail_url)}
      fullSrc={(image) => noteImageSrc(image.file_url)}
      editable={editable}
      onChanged={onChanged}
    />
  );
}
