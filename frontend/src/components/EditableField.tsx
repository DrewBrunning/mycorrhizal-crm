import { useState } from 'react';
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
  const [revealed, setRevealed] = useState(false);

  const toggleReveal = () => { setRevealed((r) => !r); };

  return (
    <Box
      sx={{
        position: 'relative',
        '&:hover .edit-icon': {
          opacity: 1
        },
        '& .copy-icon': {
          opacity: 0,
          transition: 'opacity 0.2s',
        },
        '&:hover .copy-icon': {
          opacity: 1
        }
      }}
    >
      <Box
        sx={{ display: 'flex', alignItems: multiline ? 'flex-start' : 'center' }}
        onClick={!isEditing && value ? toggleReveal : undefined}
        role={!isEditing && value ? 'button' : undefined}
        tabIndex={!isEditing && value ? 0 : undefined}
        onKeyDown={!isEditing && value ? (e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); toggleReveal(); } } : undefined}
      >
        {icon}
        <Box sx={{ flex: 1 }}>
          <Typography variant="caption" color="text.secondary">
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
            {/* Copy button hidden until hover or tap (T55), matching the edit affordance. */}
            {value && <CopyButton value={value} label={label} className={revealed ? undefined : 'copy-icon'} />}
            <IconButton
              className="edit-icon"
              size="small"
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
