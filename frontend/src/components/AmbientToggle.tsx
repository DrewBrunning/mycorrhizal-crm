import { Box, FormControlLabel, FormHelperText, Switch } from '@mui/material';
import { useTranslation } from 'react-i18next';

interface AmbientToggleProps {
  checked: boolean;
  onChange: (next: boolean) => void;
}

// Issue #1735 (backend #1733): the explicit "I see them all the time" flag,
// bound to crm.ambient. Household members are ambient automatically (derived
// server-side), so this switch only controls the explicit flag.
export default function AmbientToggle({ checked, onChange }: AmbientToggleProps) {
  const { t } = useTranslation();
  return (
    <Box sx={{ mb: 1 }}>
      <FormControlLabel
        control={
          <Switch
            checked={checked}
            onChange={(e) => onChange(e.target.checked)}
            slotProps={{ input: { 'aria-describedby': 'ambient-toggle-helper' } }}
          />
        }
        label={t('ambient.toggleLabel')}
      />
      <FormHelperText id="ambient-toggle-helper" sx={{ mt: -0.5, ml: 0 }}>
        {t('ambient.toggleHelper')}
      </FormHelperText>
    </Box>
  );
}
