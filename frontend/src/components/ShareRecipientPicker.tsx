import { Autocomplete, TextField } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { UserDirectoryEntry } from '../api/contactShares';
import { SHARE_SEARCH_MIN_CHARS, useShareRecipientSearch } from '../hooks/useShareRecipientSearch';

interface ShareRecipientPickerProps {
  label: string;
  value: UserDirectoryEntry | null;
  onChange: (value: UserDirectoryEntry | null) => void;
  onError: (message: string) => void;
}

// Recipient typeahead shared by the one-time share dialog and the standing-share form. It asks
// the server as the user types (GET /users/directory?q=), so an instance running
// SHARE_DIRECTORY_MODE=search never lists accounts; see useShareRecipientSearch.
export default function ShareRecipientPicker({
  label,
  value,
  onChange,
  onError,
}: ShareRecipientPickerProps) {
  const { t } = useTranslation();
  const { users, searchMode, query, setQuery } = useShareRecipientSearch(onError);
  // Keep the chosen user in the options so a later, narrower search never orphans the value.
  const options = value && !users.some((u) => u.id === value.id) ? [value, ...users] : users;
  const typed = query.trim().length;
  const hint = t('contactShares.recipientPicker.searchHint', { min: SHARE_SEARCH_MIN_CHARS });

  let noOptions = t('standingShares.form.noRecipients');
  if (searchMode && typed < SHARE_SEARCH_MIN_CHARS) noOptions = hint;
  else if (typed > 0) noOptions = t('contactShares.recipientPicker.noMatches');

  return (
    <Autocomplete
      options={options}
      value={value}
      onChange={(_e, next) => onChange(next)}
      onInputChange={(_e, text, reason) => {
        if (reason === 'input') setQuery(text);
        else if (reason === 'clear') setQuery('');
      }}
      getOptionLabel={(u) => u.username}
      isOptionEqualToValue={(a, b) => a.id === b.id}
      noOptionsText={noOptions}
      renderInput={(params) => (
        <TextField {...params} label={label} helperText={searchMode ? hint : undefined} />
      )}
    />
  );
}
