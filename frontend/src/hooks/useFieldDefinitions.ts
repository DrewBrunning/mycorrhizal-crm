import { useState, useCallback, useMemo } from 'react';
import {
  getFieldDefinitions,
  createFieldDefinition,
  updateFieldDefinition,
  deleteFieldDefinition,
  getContactFieldValues,
  replaceContactFieldValues,
  FieldDefinition,
  FieldDefinitionInput,
  FieldValue,
  FieldValueInput,
} from '../api/fieldDefinitions';
import { handleFetchError, handleError, ErrorNotifier } from '../utils/errorHandler';

// useFieldDefinitions drives the definition-management surface (the settings
// page's create/edit/delete) AND supplies every contact-facing view with the
// full definition list it needs to render typed editors. Definitions are
// per-user and few in number, so they are fetched once and cached in state.
export function useFieldDefinitions(notifier?: ErrorNotifier) {
  const [definitions, setDefinitions] = useState<FieldDefinition[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const response = await getFieldDefinitions(100);
      setDefinitions(response.field_definitions || []);
    } catch (err) {
      setError(handleFetchError(err, 'fetching custom field definitions'));
    } finally {
      setLoading(false);
    }
  }, []);

  const handleCreate = useCallback(
    async (input: FieldDefinitionInput) => {
      try {
        await createFieldDefinition(input);
        await refresh();
      } catch (err) {
        handleError(err, { operation: 'creating custom field' }, notifier);
        throw err;
      }
    },
    [refresh, notifier]
  );

  const handleUpdate = useCallback(
    async (id: string, input: FieldDefinitionInput) => {
      try {
        await updateFieldDefinition(id, input);
        await refresh();
      } catch (err) {
        handleError(err, { operation: 'updating custom field' }, notifier);
        throw err;
      }
    },
    [refresh, notifier]
  );

  const handleDelete = useCallback(
    async (id: string) => {
      try {
        await deleteFieldDefinition(id);
        await refresh();
      } catch (err) {
        handleError(err, { operation: 'deleting custom field' }, notifier);
        throw err;
      }
    },
    [refresh, notifier]
  );

  return {
    definitions,
    loading,
    error,
    refresh,
    handleCreate,
    handleUpdate,
    handleDelete,
  };
}

// useContactFieldValues loads and (full-)replaces one contact's FieldValues.
// contactId is the numeric contact ID the nested /contacts/:id/field-values
// routes use; a value for a definition the contact has none of is absent from
// valuesByDefinition rather than present with a null value.
export function useContactFieldValues(contactId: string | number | undefined, notifier?: ErrorNotifier) {
  const [values, setValues] = useState<FieldValue[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Accepts an optional override id so the very first load (before the
  // caller's record is resolved via state) can pass the freshly-fetched id
  // directly -- the same pattern refreshRelationshipEdges uses for its
  // overrideUid parameter, for the same reason.
  const refresh = useCallback(async (overrideId?: string | number) => {
    const id = overrideId ?? contactId;
    if (id === undefined) return;
    setLoading(true);
    setError(null);
    try {
      setValues(await getContactFieldValues(id));
    } catch (err) {
      const msg = handleFetchError(err, 'fetching custom field values');
      setError(msg);
      handleError(err, { operation: 'fetching custom field values' }, notifier);
    } finally {
      setLoading(false);
    }
  }, [contactId, notifier]);

  const valuesByDefinition = useMemo(() => {
    const map = new Map<string, unknown>();
    for (const v of values) map.set(v.field_definition_id, v.value);
    return map;
  }, [values]);

  const save = useCallback(
    async (fieldValues: FieldValueInput[]) => {
      if (contactId === undefined) return;
      try {
        setValues(await replaceContactFieldValues(contactId, fieldValues));
      } catch (err) {
        handleError(err, { operation: 'saving custom field values' }, notifier);
        throw err;
      }
    },
    [contactId, notifier]
  );

  return {
    values,
    valuesByDefinition,
    loading,
    error,
    refresh,
    save,
  };
}
