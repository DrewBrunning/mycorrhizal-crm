import { Box, Divider, Stack, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { Card } from '../api/contacts';

interface RelatedToMembersSectionProps {
  card: Card;
}

// The component renders nothing when there are no related entities or members;
// the contact page uses this to decide whether the "Card metadata" heading
// above it should render at all (T30).
export function hasRelatedToOrMembers(card: Card): boolean {
  return (card.relatedTo?.length ?? 0) > 0 || (card.members?.length ?? 0) > 0;
}

export default function RelatedToMembersSection({ card }: RelatedToMembersSectionProps) {
  const { t } = useTranslation();

  const relatedTo = card.relatedTo || [];
  const members = card.members || [];

  if (relatedTo.length === 0 && members.length === 0) return null;

  return (
    <Box sx={{ mt: 2 }}>
      <Divider sx={{ my: 1 }} />
      <Typography variant="subtitle2" component="p" gutterBottom>
        {t('contacts.relatedToMembers.title')}
      </Typography>
      <Stack spacing={1}>
        {relatedTo.length > 0 && (
          <Box>
            <Typography
              variant="caption"
              sx={{
                color: 'text.secondary',
              }}
            >
              {t('contacts.relatedToMembers.relatedTo')}
            </Typography>
            {relatedTo.map((r, i) => (
              <Typography
                // biome-ignore lint/suspicious/noArrayIndexKey: names may repeat, no stable id
                key={i}
                variant="body2"
                sx={{ overflowWrap: 'anywhere', wordBreak: 'break-word' }}
              >
                {r.target}
                {r.relations?.length ? ` (${r.relations.join(', ')})` : ''}
              </Typography>
            ))}
          </Box>
        )}
        {members.length > 0 && (
          <Box>
            <Typography
              variant="caption"
              sx={{
                color: 'text.secondary',
              }}
            >
              {t('contacts.relatedToMembers.members')}
            </Typography>
            {members.map((m, i) => (
              <Typography
                // biome-ignore lint/suspicious/noArrayIndexKey: names may repeat, no stable id
                key={i}
                variant="body2"
                sx={{ overflowWrap: 'anywhere', wordBreak: 'break-word' }}
              >
                {m}
              </Typography>
            ))}
          </Box>
        )}
      </Stack>
    </Box>
  );
}
