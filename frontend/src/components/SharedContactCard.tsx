import { Box, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { JSContactCard } from '../api/shareGrants';
import { describeJSContact } from '../utils/jscontactDisplay';

interface SharedContactCardProps {
  card: JSContactCard;
}

// SharedContactCard renders the allowlisted display model of a JSContact Card:
// the name and then only the granted section items (see utils/jscontactDisplay).
// Used by both the Share dialog's projection preview and the recipient read view.
export default function SharedContactCard({ card }: SharedContactCardProps) {
  const { t } = useTranslation();
  const { name, sections } = describeJSContact(card);
  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
      <Typography variant="subtitle1" component="p" sx={{ fontWeight: 600 }}>
        {name || t('standingShares.content.noName')}
      </Typography>
      {sections.map((section) => (
        <Box key={section.token}>
          <Typography variant="subtitle2" component="h4">
            {t(`settings.exportFieldPicker.sections.${section.token}`)}
          </Typography>
          <Box component="ul" sx={{ m: 0, pl: 2.5 }}>
            {section.items.map((item, i) => (
              // Items are display strings with no stable id; the list is static per render.
              // biome-ignore lint/suspicious/noArrayIndexKey: static, never reordered
              <li key={`${i}-${item}`}>
                <Typography variant="body2" component="span" sx={{ overflowWrap: 'anywhere' }}>
                  {item}
                </Typography>
              </li>
            ))}
          </Box>
        </Box>
      ))}
    </Box>
  );
}
