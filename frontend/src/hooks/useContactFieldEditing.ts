import { type Dispatch, type SetStateAction, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ApiError } from '../api/client';
import {
  type Card,
  type ContactRecordResponse,
  type CRMEnvelope,
  updateContactRecord,
} from '../api/contacts';
import { useDateFormat } from '../DateFormatProvider';
import { applyRecordPatch, buildRecordPatch, isDateField } from '../utils/contactDetailPayloads';

export interface ContactFieldEditingOptions {
  id: string | undefined;
  record: ContactRecordResponse | null;
  setRecord: Dispatch<SetStateAction<ContactRecordResponse | null>>;
  // The backend mirrors a wedding-anniversary change into a married LifeEvent
  // (services/wedding_sync.go); every successful save refreshes life events
  // so the timeline and the Life Events panel pick it up without a reload.
  refreshLifeEvents: (uid: string) => Promise<void>;
  showError: (message: string) => void;
}

// ContactInformation's editing surface: the one-field-at-a-time inline
// editor (with display-format <-> ISO conversion for the date fields) and the
// multi-valued / structured card updates (emails, phones, addresses, ...).
export function useContactFieldEditing({
  id,
  record,
  setRecord,
  refreshLifeEvents,
  showError,
}: ContactFieldEditingOptions) {
  const { t } = useTranslation();
  const { formatBirthdayForInput, parseBirthdayInput, autoFormatBirthdayInput } = useDateFormat();
  const [editingField, setEditingField] = useState<string | null>(null);
  const [editValue, setEditValue] = useState<string>('');
  const [validationError, setValidationError] = useState<string>('');

  const reportUpdateError = (err: unknown) => {
    console.error('Error updating contact:', err);
    if (err instanceof ApiError) {
      showError(err.getDisplayMessage());
    } else {
      showError(t('contactDetail.updateError'));
    }
  };

  const handleEditStart = (field: string, currentValue: string) => {
    setEditingField(field);
    // For date fields, convert from ISO to display format
    if (isDateField(field) && currentValue) {
      setEditValue(formatBirthdayForInput(currentValue));
    } else {
      setEditValue(currentValue || '');
    }
    setValidationError('');
  };

  const handleEditCancel = () => {
    setEditingField(null);
    setEditValue('');
    setValidationError('');
  };

  const handleEditValueChange = (value: string) => {
    setEditValue(isDateField(editingField) ? autoFormatBirthdayInput(value, editValue) : value);
    setValidationError('');
  };

  const handleEditSave = async (field: string) => {
    if (!record || !id) return;

    let valueToSave = editValue;

    if (isDateField(field)) {
      // Blank clears the date; anything else must parse in the user's
      // display format.
      if (editValue.trim() !== '' && parseBirthdayInput(editValue) === null) {
        setValidationError(t('contactDetail.birthdayError'));
        return;
      }
      // Convert from display format to ISO format for storage
      valueToSave = parseBirthdayInput(editValue) || '';
    }

    try {
      const updated = await updateContactRecord(
        id,
        applyRecordPatch(record, buildRecordPatch(record.card, field, valueToSave)),
      );
      setRecord(updated);
      setEditingField(null);
      setEditValue('');
      setValidationError('');
      await refreshLifeEvents(record.uid).catch(() => {});
    } catch (err) {
      reportUpdateError(err);
      if (err instanceof ApiError) {
        setValidationError(err.getDisplayMessage());
      }
    }
  };

  // Persist multi-valued / structured field updates (emails, phones,
  // addresses, links, imppAddresses). Rethrows so the calling editor can keep
  // its own state open on failure.
  const handleUpdateCard = async (patch: Partial<Card>, crmPatch?: Partial<CRMEnvelope>) => {
    if (!record || !id) return;
    try {
      const updated = await updateContactRecord(id, {
        gender: record.gender,
        card: { ...record.card, ...patch },
        crm: { ...record.crm, ...crmPatch },
      });
      setRecord(updated);
      await refreshLifeEvents(record.uid).catch(() => {});
    } catch (err) {
      reportUpdateError(err);
      throw err;
    }
  };

  return {
    editingField,
    editValue,
    validationError,
    handleEditStart,
    handleEditCancel,
    handleEditValueChange,
    handleEditSave,
    handleUpdateCard,
  };
}
