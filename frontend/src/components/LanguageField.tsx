import InfoOutlinedIcon from '@mui/icons-material/InfoOutlined';
import { Autocomplete, TextField, Tooltip } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { LANGUAGE_OPTIONS, type LanguageOption, languageLabel } from '../languages';

interface LanguageFieldProps {
  value: string;
  onChange: (value: string) => void;
  fullWidth?: boolean;
  size?: 'small' | 'medium';
}

// Edits Card.Language (the vCard's own language tag). It is a free-solo
// Autocomplete over the full ISO 639-1 set so BCP-47 subtags like "de-AT" can
// be typed even though only the two-letter base codes are enumerated. The info
// tooltip exists because this is card metadata, not the language the contact
// speaks — a distinction that is easy to confuse with the spoken-language
// fields (pronouns, grammatical gender, preferred languages).
export default function LanguageField({
  value,
  onChange,
  fullWidth,
  size = 'small',
}: LanguageFieldProps) {
  const { t } = useTranslation();

  return (
    <Autocomplete
      freeSolo
      size={size}
      fullWidth={fullWidth}
      options={LANGUAGE_OPTIONS}
      getOptionLabel={(opt: LanguageOption | string) =>
        typeof opt === 'string' ? opt : languageLabel(opt)
      }
      isOptionEqualToValue={(opt, val) => {
        const optCode = typeof opt === 'string' ? opt : opt.code;
        const otherCode = typeof val === 'string' ? val : (val as LanguageOption | null)?.code;
        return optCode === otherCode;
      }}
      value={value || null}
      onChange={(_, v) => {
        if (v == null) onChange('');
        else onChange(typeof v === 'string' ? v : v.code);
      }}
      renderInput={(params) => (
        <TextField
          {...params}
          label={t('contacts.language')}
          placeholder="de-AT"
          InputProps={{
            ...params.InputProps,
            endAdornment: (
              <>
                {params.InputProps.endAdornment}
                <Tooltip title={t('contacts.languageInfo')} placement="top">
                  <InfoOutlinedIcon
                    sx={{ fontSize: 18, color: 'text.secondary', cursor: 'help' }}
                  />
                </Tooltip>
              </>
            ),
          }}
        />
      )}
    />
  );
}
