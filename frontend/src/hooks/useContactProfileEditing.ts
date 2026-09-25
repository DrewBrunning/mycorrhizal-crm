import { type Dispatch, type SetStateAction, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ApiError } from '../api/client';
import { type ContactRecordResponse, updateContactRecord } from '../api/contacts';
import type { ProfileValues } from '../components/ContactHeader';
import {
  buildProfileUpdate,
  EMPTY_PROFILE_VALUES,
  profileValuesFromRecord,
} from '../utils/contactDetailPayloads';

export interface ContactProfileEditingOptions {
  id: string | undefined;
  record: ContactRecordResponse | null;
  setRecord: Dispatch<SetStateAction<ContactRecordResponse | null>>;
  showError: (message: string) => void;
}

// The ContactHeader's name/kind/language edit mode: seeded from the record on
// start, reset on cancel, saved as one PUT (see buildProfileUpdate for how the
// form maps onto Card.name without losing imported metadata).
export function useContactProfileEditing({
  id,
  record,
  setRecord,
  showError,
}: ContactProfileEditingOptions) {
  const { t } = useTranslation();
  const [editingProfile, setEditingProfile] = useState(false);
  const [profileValues, setProfileValues] = useState<ProfileValues>(EMPTY_PROFILE_VALUES);

  const handleStartEditProfile = () => {
    if (!record) return;
    setProfileValues(profileValuesFromRecord(record));
    setEditingProfile(true);
  };

  const handleCancelEditProfile = () => {
    setEditingProfile(false);
    setProfileValues(EMPTY_PROFILE_VALUES);
  };

  const handleSaveProfile = async () => {
    if (!record || !profileValues.firstname.trim()) {
      alert(t('contactDetail.firstNameRequired'));
      return;
    }
    if (!id) return;
    try {
      const updated = await updateContactRecord(id, buildProfileUpdate(record, profileValues));
      setRecord(updated);
      setEditingProfile(false);
    } catch (err) {
      console.error('Error updating profile:', err);
      if (err instanceof ApiError) {
        showError(err.getDisplayMessage());
      } else {
        showError(t('contactDetail.updateError'));
      }
    }
  };

  return {
    editingProfile,
    profileValues,
    setProfileValues,
    handleStartEditProfile,
    handleCancelEditProfile,
    handleSaveProfile,
  };
}
