import { useCallback, useState } from 'react';
import {
  createLinkFieldType,
  deleteLinkFieldType,
  getLinkFieldTypes,
  type LinkFieldType,
  type LinkFieldTypeInput,
  reorderLinkFieldTypes,
  updateLinkFieldType,
} from '../api/linkFieldTypes';
import { type ErrorNotifier, handleError, handleFetchError } from '../utils/errorHandler';

// Owns LinkFieldType list + dialog state (T34), following
// useRelationshipEdges.ts's shape. Used both read-only (ContactInformation's
// field-linking resolution) and for the full Settings CRUD screen
// (LinkFieldTypesSettings.tsx).
export function useLinkFieldTypes(notifier?: ErrorNotifier) {
  const [linkFieldTypes, setLinkFieldTypes] = useState<LinkFieldType[]>([]);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingLinkFieldType, setEditingLinkFieldType] = useState<LinkFieldType | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refreshLinkFieldTypes = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      setLinkFieldTypes(await getLinkFieldTypes());
    } catch (err) {
      setError(handleFetchError(err, 'fetching link field types'));
    } finally {
      setLoading(false);
    }
  }, []);

  const handleSaveLinkFieldType = async (input: LinkFieldTypeInput) => {
    try {
      if (editingLinkFieldType) {
        await updateLinkFieldType(editingLinkFieldType.id, input);
      } else {
        await createLinkFieldType(input);
      }
      await refreshLinkFieldTypes();
      setDialogOpen(false);
      setEditingLinkFieldType(null);
    } catch (err) {
      handleError(err, { operation: 'saving link field type' }, notifier);
      throw err;
    }
  };

  const handleEditLinkFieldType = (linkFieldType: LinkFieldType) => {
    setEditingLinkFieldType(linkFieldType);
    setDialogOpen(true);
  };

  const handleDeleteLinkFieldType = async (id: string) => {
    try {
      await deleteLinkFieldType(id);
      await refreshLinkFieldTypes();
    } catch (err) {
      handleError(err, { operation: 'deleting link field type' }, notifier);
      throw err;
    }
  };

  const handleAddLinkFieldType = () => {
    setEditingLinkFieldType(null);
    setDialogOpen(true);
  };

  // Swaps the given item with its immediate neighbor (direction -1 = up,
  // +1 = down) within its own category group, then persists the full
  // resulting order in one call — no drag-and-drop library exists in this
  // repo yet, and one type registry doesn't warrant adding one.
  const handleMoveLinkFieldType = async (id: string, direction: -1 | 1) => {
    const item = linkFieldTypes.find((lt) => lt.id === id);
    if (!item) return;
    const groupIds = linkFieldTypes
      .filter((lt) => lt.category === item.category)
      .map((lt) => lt.id);
    const index = groupIds.indexOf(id);
    const swapIndex = index + direction;
    if (swapIndex < 0 || swapIndex >= groupIds.length) return;
    [groupIds[index], groupIds[swapIndex]] = [groupIds[swapIndex], groupIds[index]];

    // Reorder is a full-set call — splice the reordered group back into the
    // complete ID list, preserving every other category's relative order.
    const fullOrder: string[] = [];
    let groupCursor = 0;
    for (const lt of linkFieldTypes) {
      if (lt.category === item.category) {
        fullOrder.push(groupIds[groupCursor]);
        groupCursor++;
      } else {
        fullOrder.push(lt.id);
      }
    }

    try {
      setLinkFieldTypes(await reorderLinkFieldTypes(fullOrder));
    } catch (err) {
      handleError(err, { operation: 'reordering link field types' }, notifier);
      throw err;
    }
  };

  return {
    linkFieldTypes,
    dialogOpen,
    editingLinkFieldType,
    loading,
    error,
    refreshLinkFieldTypes,
    handleSaveLinkFieldType,
    handleEditLinkFieldType,
    handleDeleteLinkFieldType,
    handleAddLinkFieldType,
    handleMoveLinkFieldType,
    setDialogOpen,
    setEditingLinkFieldType,
  };
}
