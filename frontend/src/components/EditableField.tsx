import { Box, Typography, TextField, IconButton } from '@mui/material';
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
  onEditValueChange
}: EditableFieldProps) {
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
          <Typography variant="caption" color="text.secondary" sx={{ fontFamily: '"IBM Plex Mono", monospace' }}>
            {label}
          </Typography>
          {isEditing ? (
            <Box>
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mt: 0.5 }}>
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
                <IconButton size="small" color="primary" onClick={() => onEditSave(field)}>
                  <SaveIcon fontSize="small" />
                </IconButton>
                <IconButton size="small" onClick={onEditCancel}>
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
        {!isEditing && (
          <Box sx={{ display: 'flex', alignItems: 'center', ml: 1 }}>
            {value && <CopyButton value={value} label={label} />}
            <IconButton
              className="edit-icon"
              size="small"
              color="primary"
              onClick={() => onEditStart(field, value)}
              sx={{
                opacity: 0,
                transition: 'opacity 0.2s',
              }}
            >
              <EditIcon fontSize="small" />
            </IconButton>
          </Box>
        )}
      </Box>
    </Box>
  );
}
