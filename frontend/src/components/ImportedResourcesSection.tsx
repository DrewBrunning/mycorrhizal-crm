import { useTranslation } from 'react-i18next';
import { Box, Typography, Stack, Divider } from '@mui/material';
import { Card, CardResource } from '../api/contacts';

interface ImportedResourcesSectionProps {
  card: Card;
}

// Maps each read-only resource field to its display label i18n key. These are
// the low-usage, high-importance vCard extension properties (WP7): they get
// full editing UI in a later ticket (T29b), but must be *visible* and
// round-trip-preserved here so an imported contact never loses them on the
// next UI edit-and-save.
const RESOURCE_FIELDS: Array<{ field: keyof Card; labelKey: string }> = [
  { field: 'media', labelKey: 'contacts.importedResources.media' },
  { field: 'calendars', labelKey: 'contacts.importedResources.calendars' },
  { field: 'freeBusyUrls', labelKey: 'contacts.importedResources.freeBusyUrls' },
  { field: 'schedulingAddresses', labelKey: 'contacts.importedResources.schedulingAddresses' },
  { field: 'cryptoKeys', labelKey: 'contacts.importedResources.cryptoKeys' },
  { field: 'directories', labelKey: 'contacts.importedResources.directories' },
  { field: 'contactUris', labelKey: 'contacts.importedResources.contactUris' },
];

// The component renders nothing when every resource list is empty; the contact
// page uses this to decide whether the "Card metadata" heading above it should
// render at all (T30) — an orphan heading is the same bug as a hidden section.
export function hasImportedResources(card: Card): boolean {
  return RESOURCE_FIELDS.some(({ field }) => {
    const v = card[field];
    return Array.isArray(v) && v.length > 0;
  });
}

function renderResource(r: CardResource): string {
  const parts = [r.uri];
  if (r.kind) parts.push(r.kind);
  if (r.mediaType) parts.push(r.mediaType);
  if (r.label) parts.push(r.label);
  if (r.contexts?.length) parts.push(`(${r.contexts.join(', ')})`);
  return parts.join(' · ');
}

export default function ImportedResourcesSection({ card }: ImportedResourcesSectionProps) {
  const { t } = useTranslation();

  const fields = RESOURCE_FIELDS.filter(({ field }) => {
    const v = card[field];
    return Array.isArray(v) && v.length > 0;
  });

  if (fields.length === 0) return null;

  return (
    <Box sx={{ mt: 2 }}>
      <Divider sx={{ my: 1 }} />
      <Typography variant="subtitle2" gutterBottom>
        {t('contacts.importedResources.title')}
      </Typography>
      <Stack spacing={1}>
        {fields.map(({ field, labelKey }) => (
          <Box key={field}>
            <Typography variant="caption" color="text.secondary">
              {t(labelKey)}
            </Typography>
            <Stack>
              {(card[field] as CardResource[]).map((r, i) => (
                <Typography key={i} variant="body2" sx={{ overflowWrap: 'anywhere', wordBreak: 'break-word' }}>
                  {renderResource(r)}
                </Typography>
              ))}
            </Stack>
          </Box>
        ))}
      </Stack>
    </Box>
  );
}
