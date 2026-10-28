import { Autocomplete, Box, Chip, TextField, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { useCircles } from '../hooks/useCircles';
import { useTags } from '../hooks/useTags';

// Issue #2000: an option is an existing Circle/Tag, or a not-yet-created name
// (create-new) that is created when the user picks it.
export interface FixedGroupOption {
  id: string;
  name: string;
}

interface Props {
  circleIds: string[];
  tagIds: string[];
  onChange: (next: { circleIds: string[]; tagIds: string[] }) => void;
}

const CREATE_PREFIX = '__create__:';

interface GroupPickerProps {
  label: string;
  options: FixedGroupOption[];
  selectedIds: string[];
  onSelectedIds: (ids: string[]) => void;
  onCreate: (name: string) => Promise<FixedGroupOption>;
  addLabel: (name: string) => string;
}

function GroupPicker({
  label,
  options,
  selectedIds,
  onSelectedIds,
  onCreate,
  addLabel,
}: GroupPickerProps) {
  const selected = options.filter((o) => selectedIds.includes(o.id));
  return (
    <Autocomplete
      multiple
      size="small"
      options={options}
      value={selected}
      getOptionLabel={(o) => o.name}
      isOptionEqualToValue={(a, b) => a.id === b.id}
      filterOptions={(opts, state) => {
        const q = state.inputValue.trim();
        const filtered = opts.filter((o) => o.name.toLowerCase().includes(q.toLowerCase()));
        const exact = opts.some((o) => o.name.toLowerCase() === q.toLowerCase());
        if (q && !exact) filtered.push({ id: `${CREATE_PREFIX}${q}`, name: addLabel(q) });
        return filtered;
      }}
      onChange={(_, next) => {
        const create = next.find((o) => o.id.startsWith(CREATE_PREFIX));
        const existing = next.filter((o) => !o.id.startsWith(CREATE_PREFIX)).map((o) => o.id);
        if (!create) {
          onSelectedIds(existing);
          return;
        }
        void onCreate(create.id.slice(CREATE_PREFIX.length))
          .then((created) => onSelectedIds([...existing, created.id]))
          .catch(() => onSelectedIds(existing));
      }}
      renderValue={(value, getItemProps) =>
        value.map((o, index) => {
          const { key, ...itemProps } = getItemProps({ index });
          return <Chip key={key} label={o.name} size="small" {...itemProps} />;
        })
      }
      renderInput={(params) => <TextField {...params} label={label} />}
    />
  );
}

export default function ImportFixedAssignmentPicker({ circleIds, tagIds, onChange }: Props) {
  const { t } = useTranslation();
  const { circles, refresh: refreshCircles, handleCreate: createCircleOption } = useCircles();
  const { tags, refresh: refreshTags, handleCreate: createTagOption } = useTags();

  // Refresh after a create so the new entity is in `options` when it is selected.
  const createCircle = async (name: string) => {
    const created = await createCircleOption(name);
    await refreshCircles();
    return created;
  };
  const createTag = async (name: string) => {
    const created = await createTagOption(name);
    await refreshTags();
    return created;
  };

  return (
    <Box sx={{ mb: 2, display: 'flex', flexDirection: 'column', gap: 1.5 }}>
      <Typography variant="subtitle2">
        {t('contacts.import.fixedAssignment.title', 'Add every imported contact to')}
      </Typography>
      <Typography variant="body2" sx={{ color: 'text.secondary' }}>
        {t(
          'contacts.import.fixedAssignment.description',
          'Optional. These circles and tags are applied to every contact you import, in addition to any circles or tags mapped from columns.',
        )}
      </Typography>
      <GroupPicker
        label={t('contacts.import.fixedAssignment.circles', 'Circles')}
        options={circles}
        selectedIds={circleIds}
        onSelectedIds={(ids) => onChange({ circleIds: ids, tagIds })}
        onCreate={createCircle}
        addLabel={(name) =>
          t('contacts.import.fixedAssignment.create', 'Create "{{name}}"', { name })
        }
      />
      <GroupPicker
        label={t('contacts.import.fixedAssignment.tags', 'Tags')}
        options={tags}
        selectedIds={tagIds}
        onSelectedIds={(ids) => onChange({ circleIds, tagIds: ids })}
        onCreate={createTag}
        addLabel={(name) =>
          t('contacts.import.fixedAssignment.create', 'Create "{{name}}"', { name })
        }
      />
    </Box>
  );
}
