import { useCallback, useState } from 'react';

// The create-or-edit dialog state shape every panel on ContactDetailPage
// repeated by hand: one `open` flag plus the item being edited (`null` when
// the dialog is creating a new one). openCreate/openEdit/close are stable, so
// they are safe to hand straight to a list component's onAdd/onEdit props.
export function useEditDialog<T>() {
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState<T | null>(null);

  const openCreate = useCallback(() => {
    setEditing(null);
    setOpen(true);
  }, []);

  const openEdit = useCallback((item: T) => {
    setEditing(item);
    setOpen(true);
  }, []);

  // Closing also clears the edited item, so the next openCreate can never
  // render the previous edit's values for a frame.
  const close = useCallback(() => {
    setOpen(false);
    setEditing(null);
  }, []);

  return { open, editing, openCreate, openEdit, close };
}
