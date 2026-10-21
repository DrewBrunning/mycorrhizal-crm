import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import { Accordion, AccordionDetails, AccordionSummary, Box, Typography } from '@mui/material';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { FieldDefinition } from '../api/fieldDefinitions';
import { groupDefinitions, hasAnyGroup, isGroupFilled } from '../utils/customFieldGroups';
import CustomFieldValueRow from './CustomFieldValueRow';

interface CustomFieldGroupsProps {
  definitions: FieldDefinition[];
  valuesByDefinition?: Map<string, unknown>;
  onSave: (definitionId: string, value: unknown) => Promise<void>;
}

// Renders a contact's custom fields (issue #1732). With no grouped definition
// the rows render flat, exactly as before. Otherwise there is one collapsible
// accordion per group label plus a trailing "Custom fields" accordion for the
// ungrouped ones; a group with no filled value starts collapsed. This lives
// inside the General Information card, so it needs no SectionGroup id or
// ContactJumpNav entry of its own.
export default function CustomFieldGroups({
  definitions,
  valuesByDefinition,
  onSave,
}: CustomFieldGroupsProps) {
  const { t, i18n } = useTranslation();
  // Expansion is controlled rather than `defaultExpanded`: definitions and
  // values load independently, so a group can mount empty and receive its
  // value a render later. `defaultExpanded` is read once at mount, which left
  // such groups permanently collapsed. An explicit user toggle wins.
  const [expandedGroups, setExpandedGroups] = useState<Record<string, boolean>>({});

  const renderRows = (defs: FieldDefinition[]) =>
    defs.map((definition) => (
      <CustomFieldValueRow
        key={definition.id}
        definition={definition}
        value={valuesByDefinition?.get(definition.id)}
        onSave={onSave}
      />
    ));

  if (!hasAnyGroup(definitions)) return <>{renderRows(definitions)}</>;

  return (
    <Box sx={{ gridColumn: { lg: '1 / -1' } }}>
      {groupDefinitions(definitions, i18n.language).map((g) => {
        const title = g.group || t('contactDetail.section.customFieldsUngrouped');
        const id = `custom-field-group-${g.group === '' ? 'ungrouped' : encodeURIComponent(g.group)}`;
        const groupKey = g.group === '' ? '\u0000ungrouped' : g.group;
        return (
          <Accordion
            key={groupKey}
            expanded={expandedGroups[groupKey] ?? isGroupFilled(g, valuesByDefinition)}
            onChange={(_, isExpanded) =>
              setExpandedGroups((prev) => ({ ...prev, [groupKey]: isExpanded }))
            }
            disableGutters
            variant="outlined"
          >
            <AccordionSummary
              expandIcon={<ExpandMoreIcon />}
              aria-controls={`${id}-content`}
              id={`${id}-header`}
            >
              <Typography variant="subtitle2">{title}</Typography>
            </AccordionSummary>
            <AccordionDetails id={`${id}-content`} sx={{ display: 'grid', gap: 2 }}>
              {renderRows(g.definitions)}
            </AccordionDetails>
          </Accordion>
        );
      })}
    </Box>
  );
}
