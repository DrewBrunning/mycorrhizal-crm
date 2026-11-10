import { Autocomplete, TextField } from '@mui/material';
import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { type Entity, type EntityKind, listEntities } from '../api/entities';
import { useDebouncedValue } from '../hooks/useDebounce';

export type EntityChoice = { create: false; entity: Entity } | { create: true; name: string };

interface EntityPickerProps {
  kind: EntityKind;
  value: EntityChoice | null;
  onChange: (value: EntityChoice | null) => void;
  /** Hide the "Create …" option (the caller only wants existing entities). */
  disableCreate?: boolean;
}

type Option = { id: string; label: string; choice: EntityChoice };

// Search-as-you-type picker over the caller's entities of one kind, with a
// trailing "Create “<typed name>”" option. Matching is the server's (`q` is
// folded with the same normalization as the list route).
export default function EntityPicker({ kind, value, onChange, disableCreate }: EntityPickerProps) {
  const { t } = useTranslation();
  const [input, setInput] = useState(value && !value.create ? value.entity.name : '');
  const [found, setFound] = useState<Entity[]>([]);
  const debounced = useDebouncedValue(input.trim(), 250);

  useEffect(() => {
    let cancelled = false;
    listEntities({ kind, q: debounced || undefined, limit: 20 })
      .then((page) => {
        if (!cancelled) setFound(page.entities);
      })
      .catch(() => {
        if (!cancelled) setFound([]);
      });
    return () => {
      cancelled = true;
    };
  }, [kind, debounced]);

  const options = useMemo<Option[]>(() => {
    const list: Option[] = found.map((e) => ({
      id: e.id,
      label: e.name,
      choice: { create: false, entity: e },
    }));
    const typed = input.trim();
    if (
      !disableCreate &&
      typed &&
      !found.some((e) => e.name.toLowerCase() === typed.toLowerCase())
    ) {
      list.push({
        id: `create:${typed}`,
        label: t('contactDetail.entityLinks.create', { name: typed }),
        choice: { create: true, name: typed },
      });
    }
    return list;
  }, [found, input, disableCreate, t]);

  const selected: Option | null = value
    ? value.create
      ? {
          id: `create:${value.name}`,
          label: t('contactDetail.entityLinks.create', { name: value.name }),
          choice: value,
        }
      : { id: value.entity.id, label: value.entity.name, choice: value }
    : null;

  return (
    <Autocomplete
      options={options}
      value={selected}
      inputValue={input}
      onInputChange={(_, v, reason) => {
        if (reason === 'reset') {
          // Selecting an option resets the text to its label; for the "Create" option
          // keep the typed name rather than the "Create “…”" caption.
          return;
        }
        setInput(v);
      }}
      onChange={(_, option) => {
        onChange(option?.choice ?? null);
        if (option) setInput(option.choice.create ? option.choice.name : option.choice.entity.name);
      }}
      filterOptions={(x) => x}
      getOptionLabel={(o) => o.label}
      isOptionEqualToValue={(a, b) => a.id === b.id}
      renderInput={(params) => (
        <TextField {...params} label={t('contactDetail.entityLinks.entity')} required />
      )}
    />
  );
}
