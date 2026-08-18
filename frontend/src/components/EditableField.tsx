import { Box, Typography, TextField, IconButton, Autocomplete } from '@mui/material';
import { useTranslation } from 'react-i18next';
import SaveIcon from '@mui/icons-material/Save';
import CloseIcon from '@mui/icons-material/Close';
import EditIcon from '@mui/icons-material/Edit';
import CopyButton from './CopyButton';

interface EditableFieldProps {
  icon: React.ReactNode;
  label: string;
  field: string;
  value: string;
  multiline?: boolean;
  placeholder?: string;
  displaySuffix?: string;
  formattedDisplayValue?: string; // Optional formatted value for display (raw value used for editing)
  isEditing: boolean;
  editValue: string;
  validationError: string;
  onEditStart: (field: string, value: string) => void;
  onEditCancel: () => void;
  onEditSave: (field: string) => void;
  onEditValueChange: (value: string) => void;
  // T72: when set, edit mode renders a free-solo Autocomplete offering these
  // suggestions instead of a plain TextField. Free text is still accepted and
  // saved verbatim — this is a suggestion list, not a constrained enum. Opt-in
  // so every other EditableField consumer (birthday, organization, ...) is
  // unaffected.
  options?: string[];
  getOptionLabel?: (option: string) => string;
}

export default function EditableField({
  icon,
  label,
  field,
  value,
  multiline = false,
  placeholder = '',
  displaySuffix,
  formattedDisplayValue,
  isEditing,
  editValue,
  validationError,
  onEditStart,
  onEditCancel,
  onEditSave,
  onEditValueChange,
  options,
  getOptionLabel
}: EditableFieldProps) {
  const { t } = useTranslation();
  const baseDisplayValue = formattedDisplayValue || value;
  const displayValue = baseDisplayValue ? (displaySuffix ? `${baseDisplayValue} ${displaySuffix}` : baseDisplayValue) : '-';
  const showError = isEditing && validationError;

  return (
    <Box
      sx={{
        position: 'relative',
        '&:hover .edit-icon': {
          opacity: 1
        }
      }}
    >
      <Box
        sx={{ display: 'flex', alignItems: multiline ? 'flex-start' : 'center' }}
      >
        {icon}
        {/* T74: minWidth: 0 lets this shrink below its content's natural width
            inside a narrower grid cell (a flex child defaults to min-width:
            auto) -- EditableArrayField's equivalent box already had this. */}
        <Box sx={{ flex: 1, minWidth: 0 }}>
          {/* T63: field-name label gets IBM Plex Mono to contrast against the
              IBM Plex Sans field value below it -- component-scoped since
              "caption" is reused 60+ times elsewhere (timestamps, hints,
              error text) and isn't safe to retheme globally. */}
          {/* T109: the edit pencil sits beside the field-name label, matching
              Name and Circles/Tags (T89), not at the far right of the row.
              The label row is a flex sibling so the pencil rides the caption
              baseline; the value keeps its own row below. */}
          <Box sx={{ display: 'flex', alignItems: 'center' }}>
            <Typography variant="caption" color="text.secondary" sx={{ fontFamily: '"IBM Plex Mono", monospace' }}>
              {label}
            </Typography>
            {!isEditing && (
              <IconButton
                className="edit-icon"
                size="small"
                color="primary"
                onClick={() => onEditStart(field, value)}
                aria-label={t('common.edit')}
                sx={{ ml: 0.5, p: 0.25, opacity: 0, transition: 'opacity 0.2s' }}
              >
                <EditIcon sx={{ fontSize: 18 }} />
              </IconButton>
            )}
          </Box>
          {isEditing ? (
            <Box>
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mt: 0.5 }}>
                {options ? (
                  <Autocomplete
                    freeSolo
                    fullWidth
                    size="small"
                    options={options}
                    getOptionLabel={getOptionLabel ?? ((option) => option)}
                    value={editValue || null}
                    onChange={(_, newValue) => onEditValueChange(newValue || '')}
                    onInputChange={(_, newValue) => onEditValueChange(newValue)}
                    renderInput={(params) => (
                      <TextField
                        {...params}
                        autoFocus
                        error={!!showError}
                        placeholder={placeholder}
                      />
                    )}
                  />
                ) : (
                  <TextField
                    value={editValue}
                    onChange={(e) => onEditValueChange(e.target.value)}
                    size="small"
                    fullWidth
                    multiline={multiline}
                    rows={multiline ? 3 : 1}
                    autoFocus
                    error={!!showError}
                    placeholder={placeholder}
                  />
                )}
                <IconButton size="small" color="primary" onClick={() => onEditSave(field)} aria-label={t('common.save')}>
                  <SaveIcon fontSize="small" />
                </IconButton>
                <IconButton size="small" onClick={onEditCancel} aria-label={t('common.cancel')}>
                  <CloseIcon fontSize="small" />
                </IconButton>
              </Box>
              {showError && (
                <Typography variant="caption" color="error" sx={{ mt: 0.5, display: 'block' }}>
                  {validationError}
                </Typography>
              )}
            </Box>
          ) : (
            <Typography variant="body1" sx={{ overflowWrap: 'anywhere', wordBreak: 'break-word', whiteSpace: multiline ? 'pre-wrap' : undefined }}>
              {displayValue}
            </Typography>
          )}
        </Box>
        {!isEditing && value && (
          <Box sx={{ display: 'flex', alignItems: 'center', ml: 1 }}>
            <CopyButton value={value} label={label} />
          </Box>
        )}
      </Box>
    </Box>
  );
}
