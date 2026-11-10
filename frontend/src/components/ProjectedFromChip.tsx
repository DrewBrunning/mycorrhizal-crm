import { Chip, Tooltip } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';
import { type ContactEntityLink, entityLinkIdFromElementId } from '../api/entities';

interface ProjectedFromChipProps {
  /** The ORG/ADR card entry's element id; only `ent-<linkId>` ids render a chip. */
  elementId: string | undefined;
  entityLinks: ContactEntityLink[] | undefined;
}

// ADR 0043 decision 10: a projected value is never shown without a "from
// <entity>" chip, so a user can tell it from a value the contact owns. Render
// it in a Box next to the value, never inside a <Typography> (frontend trap 3).
export default function ProjectedFromChip({ elementId, entityLinks }: ProjectedFromChipProps) {
  const { t } = useTranslation();
  const linkId = entityLinkIdFromElementId(elementId);
  if (!linkId) return null;
  const source = entityLinks?.find((l) => l.id === linkId);
  const label = source
    ? t('contactDetail.entityLinks.projectedFrom', { name: source.entity_name })
    : t('contactDetail.entityLinks.projectedFromUnknown');
  return (
    <Tooltip title={t('contactDetail.entityLinks.projectedHint')} describeChild>
      {source ? (
        <Chip
          size="small"
          variant="outlined"
          color="primary"
          label={label}
          component={Link}
          to={`/entities/${source.entity_id}`}
          clickable
        />
      ) : (
        <Chip size="small" variant="outlined" color="primary" label={label} />
      )}
    </Tooltip>
  );
}
