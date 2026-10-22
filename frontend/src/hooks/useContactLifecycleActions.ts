import type { Dispatch, SetStateAction } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import { ApiError, isPreconditionFailed } from '../api/client';
import {
  archiveContact,
  type ContactRecordResponse,
  deleteContact,
  favoriteContact,
  getContactRecord,
  unarchiveContact,
  unfavoriteContact,
} from '../api/contacts';
import {
  type ContactTargetClosenessResponse,
  clearTargetCloseness,
  setTargetCloseness,
  type TargetClosenessTier,
} from '../api/targetCloseness';
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
  // Called when the delete is rejected with 412 (the contact changed
  // elsewhere since it was loaded; ADR 0008).
  onConflict?: () => void;
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
  onConflict,
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

  // ADR 0008: favorite/archive PATCHes persist the contact, which bumps its
  // revision, but their flat response carries none. Re-read it so the next
  // save/delete sends the current If-Match instead of 412ing against our own
  // toggle. Adopt the fresh record wholesale: merging only the revision onto
  // the old card would pair a current If-Match with stale data and silently
  // overwrite a concurrent remote edit. The GET is authoritative and already
  // reflects the archive/favorite flip. Best-effort: on failure the held
  // record just stays as it was.
  const syncRevision = async () => {
    if (!id) return;
    try {
      const fresh = await getContactRecord(id);
      setRecord((prev) => (prev ? fresh : prev));
    } catch (err) {
      console.error('Error refreshing contact revision:', err);
    }
  };

  const handleDeleteContact = async () => {
    if (!record || !id) return;
    if (!window.confirm(t('contactDetail.confirmDeleteContact', { name: displayName }))) {
      return;
    }
    try {
      await deleteContact(id, record.revision);
      await navigate('/contacts');
    } catch (err) {
      console.error('Error deleting contact:', err);
      if (onConflict && isPreconditionFailed(err)) {
        onConflict();
        return;
      }
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
      await syncRevision();
    } catch (err) {
      reportError('Error archiving contact:', err);
    }
  };

  const handleUnarchiveContact = async () => {
    if (!record || !id) return;
    try {
      const updatedContact = await unarchiveContact(id);
      setRecord({ ...record, archived: updatedContact.archived });
      await syncRevision();
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
      await syncRevision();
    } catch (err) {
      setRecord((prev) => (prev ? { ...prev, is_favorite: wasFavorite } : prev));
      reportError('Error toggling favorite:', err);
    }
  };

  // Issue #1796/#1797: set or clear the contact's target closeness. The write
  // bumps the revision like favorite/archive, so re-read it before the next
  // full-record save would 412. Returns the response (with the cadence-policy
  // offer, if any) so the header can open the offer dialog; null on failure.
  const handleSetTargetCloseness = async (
    tier: TargetClosenessTier | null,
  ): Promise<ContactTargetClosenessResponse | null> => {
    if (!record || !id) return null;
    try {
      const response =
        tier === null ? await clearTargetCloseness(id) : await setTargetCloseness(id, tier);
      setRecord((prev) =>
        prev
          ? {
              ...prev,
              crm: { ...prev.crm, target_closeness: response.target_closeness ?? undefined },
            }
          : prev,
      );
      await syncRevision();
      return response;
    } catch (err) {
      reportError('Error setting target closeness:', err);
      return null;
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
    handleSetTargetCloseness,
    handleToggleMe,
  };
}
