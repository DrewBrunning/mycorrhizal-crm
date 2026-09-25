import type { Dispatch, SetStateAction } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import { ApiError } from '../api/client';
import {
  archiveContact,
  type ContactRecordResponse,
  deleteContact,
  favoriteContact,
  unarchiveContact,
  unfavoriteContact,
} from '../api/contacts';
import { updateSelfContact } from '../api/users';
import { fetchAndCacheUserInfo } from '../auth';

export interface ContactLifecycleOptions {
  id: string | undefined;
  record: ContactRecordResponse | null;
  setRecord: Dispatch<SetStateAction<ContactRecordResponse | null>>;
  // Display name for the delete confirmation ("First Last").
  displayName: string;
  selfContactUid: string | null;
  setSelfContactUid: (uid: string | null) => void;
  showError: (message: string) => void;
  showSuccess: (message: string) => void;
}

// The header's whole-contact actions: delete, archive/unarchive, favorite,
// and the T90 "this is me" toggle.
export function useContactLifecycleActions({
  id,
  record,
  setRecord,
  displayName,
  selfContactUid,
  setSelfContactUid,
  showError,
  showSuccess,
}: ContactLifecycleOptions) {
  const { t } = useTranslation();
  const navigate = useNavigate();

  const reportError = (context: string, err: unknown) => {
    console.error(context, err);
    if (err instanceof ApiError) {
      showError(err.getDisplayMessage());
    } else {
      showError(t('contactDetail.updateError'));
    }
  };

  const handleDeleteContact = async () => {
    if (!record || !id) return;
    if (!window.confirm(t('contactDetail.confirmDeleteContact', { name: displayName }))) {
      return;
    }
    try {
      await deleteContact(id);
      await navigate('/contacts');
    } catch (err) {
      console.error('Error deleting contact:', err);
      alert(t('contactDetail.deleteContactError'));
    }
  };

  // Archive/unarchive keep the rest of the loaded record and only take the
  // server's archived flag.
  const handleArchiveContact = async () => {
    if (!record || !id) return;
    if (!window.confirm(t('contactDetail.archiveConfirmation'))) {
      return;
    }
    try {
      const updatedContact = await archiveContact(id);
      setRecord({ ...record, archived: updatedContact.archived });
    } catch (err) {
      reportError('Error archiving contact:', err);
    }
  };

  const handleUnarchiveContact = async () => {
    if (!record || !id) return;
    try {
      const updatedContact = await unarchiveContact(id);
      setRecord({ ...record, archived: updatedContact.archived });
    } catch (err) {
      reportError('Error unarchiving contact:', err);
    }
  };

  // Issue #173: optimistic into state, with a rollback on failure so the star
  // can never silently disagree with the database -- unlike the archive
  // handlers above, which predate this pattern.
  const handleToggleFavorite = async () => {
    if (!record || !id) return;
    const wasFavorite = !!record.is_favorite;
    setRecord((prev) => (prev ? { ...prev, is_favorite: !wasFavorite } : prev));
    try {
      const updatedContact = wasFavorite ? await unfavoriteContact(id) : await favoriteContact(id);
      setRecord((prev) => (prev ? { ...prev, is_favorite: updatedContact.is_favorite } : prev));
    } catch (err) {
      setRecord((prev) => (prev ? { ...prev, is_favorite: wasFavorite } : prev));
      reportError('Error toggling favorite:', err);
    }
  };

  // T90: set/clear the caller's "Me" pointer. The PATCH commits the exact
  // value computed here, so reflect it immediately (no extra /users/me
  // round-trip), then refresh the localStorage cache so the Settings picker
  // and any other page agree without a reload. fetchAndCacheUserInfo swallows
  // its own fetch errors and returns null, which is why the optimistic set
  // happens first -- a failed cache refresh must not roll the badge back.
  const handleToggleMe = async () => {
    if (!record) return;
    try {
      const willBeMe = selfContactUid !== record.uid;
      const newUid = willBeMe ? record.uid : null;
      await updateSelfContact(newUid);
      setSelfContactUid(newUid);
      await fetchAndCacheUserInfo();
      showSuccess(
        willBeMe ? t('settings.selfContact.saveSuccess') : t('settings.selfContact.clearSuccess'),
      );
    } catch (err) {
      console.error('Error updating self contact:', err);
      showError(t('settings.selfContact.saveError'));
    }
  };

  return {
    handleDeleteContact,
    handleArchiveContact,
    handleUnarchiveContact,
    handleToggleFavorite,
    handleToggleMe,
  };
}
