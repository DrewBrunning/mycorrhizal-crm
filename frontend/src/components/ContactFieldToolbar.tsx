import AddIcon from '@mui/icons-material/Add';
import TuneIcon from '@mui/icons-material/Tune';
import { Box, Button, Menu, MenuItem } from '@mui/material';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { CONTACT_FIELDS, type ContactFieldKey } from '../contactFields';

// The two display affordances above a contact's fields (issue #1997):
//   - "Add a field": hide-empty mode suppresses fields with no value, so this
//     menu is how the user still reaches them. It lists exactly the enabled
//     fields hide-empty is hiding and renders nothing when there are none.
//   - "Customize fields": opens the per-contact show/hide dialog.
// Cosmetic display controls only.

interface Props {
  hiddenEmptyFields: ContactFieldKey[];
  onReveal: (key: ContactFieldKey) => void;
  onCustomize: () => void;
}

export default function ContactFieldToolbar({ hiddenEmptyFields, onReveal, onCustomize }: Props) {
  const { t } = useTranslation();
  const [anchor, setAnchor] = useState<HTMLElement | null>(null);

  const labelFor = (key: ContactFieldKey) => {
    const def = CONTACT_FIELDS.find((f) => f.key === key);
    return def ? t(def.labelKey) : key;
  };

  return (
    <Box sx={{ display: 'flex', justifyContent: 'flex-end', flexWrap: 'wrap', gap: 1, mb: 1 }}>
      {hiddenEmptyFields.length > 0 && (
        <>
          <Button
            size="small"
            startIcon={<AddIcon />}
            aria-haspopup="menu"
            aria-expanded={anchor ? 'true' : undefined}
            onClick={(e) => setAnchor(e.currentTarget)}
          >
            {t('contactDetail.fieldVisibility.addField')}
          </Button>
          <Menu anchorEl={anchor} open={!!anchor} onClose={() => setAnchor(null)}>
            {hiddenEmptyFields.map((key) => (
              <MenuItem
                key={key}
                onClick={() => {
                  setAnchor(null);
                  onReveal(key);
                }}
              >
                {labelFor(key)}
              </MenuItem>
            ))}
          </Menu>
        </>
      )}
      <Button size="small" startIcon={<TuneIcon />} onClick={onCustomize}>
        {t('contactDetail.fieldVisibility.customize')}
      </Button>
    </Box>
  );
}
