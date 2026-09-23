// Accessible text/list alternative to NetworkGraph's canvas (#189, WCAG
// 1.1.1 / 2.1.1). Always rendered in the DOM -- never behind a toggle that
// starts hidden -- so every contact in the graph is keyboard-reachable and
// has a text description of its connections, independent of the canvas.
//
// Renders from the exact same computeFilteredGraphData output NetworkGraph
// draws from (see networkGraphData.ts), so the list and the graph can never
// disagree about what's currently visible under the five filters.
import {
  Box,
  List,
  ListItem,
  ListItemButton,
  ListItemText,
  Typography,
  useTheme,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { RELATIONSHIP_EDGE_TYPES, type RelationshipEdgeType } from '../api/relationshipEdges';
import type { GraphEdge, GraphNode } from '../types/graph';
import { healthBandColor } from '../utils/healthBand';
import { edgeEndpointId } from '../utils/networkGraphData';

interface NetworkListViewProps {
  nodes: GraphNode[];
  links: GraphEdge[];
  onContactClick: (node: GraphNode) => void;
}

export default function NetworkListView({ nodes, links, onContactClick }: NetworkListViewProps) {
  const { t } = useTranslation();
  const theme = useTheme();
  const nodesById = new Map(nodes.map((n) => [n.id, n]));
  const contactNodes = nodes
    .filter((n) => n.type === 'contact')
    .sort((a, b) => a.label.localeCompare(b.label));

  // Relation tokens describe the SOURCE's role relative to the TARGET (see
  // relationship_type_registry.go / relationshipEdges.ts's own direction
  // note). When the contact we're describing is the edge's target, the label
  // needs to be inverted to read from *this* contact's perspective.
  const relationLabelFor = (contactId: string, sourceId: string, rawType: string): string => {
    const meta = RELATIONSHIP_EDGE_TYPES[rawType as RelationshipEdgeType];
    const effectiveType = sourceId === contactId ? rawType : (meta?.inverse ?? 'related_to');
    return t(`relationships.types.${effectiveType}`, effectiveType);
  };

  const describeConnections = (contact: GraphNode): string[] => {
    const descriptions: string[] = [];

    links.forEach((link) => {
      const sourceId = edgeEndpointId(link.source);
      const targetId = edgeEndpointId(link.target);
      if (sourceId !== contact.id && targetId !== contact.id) return;

      if (link.type === 'relationship') {
        const otherId = sourceId === contact.id ? targetId : sourceId;
        const otherNode = nodesById.get(otherId);
        if (!otherNode) return;
        descriptions.push(
          t('network.connectionRelationship', {
            type: relationLabelFor(contact.id, sourceId, link.label),
            name: otherNode.label,
          }),
        );
      } else if (link.type === 'activity' && targetId === contact.id) {
        descriptions.push(t('network.connectionActivity', { title: link.label }));
      } else if (link.type === 'circle' && sourceId === contact.id) {
        descriptions.push(t('network.connectionCircle', { circle: link.label }));
      }
    });

    return descriptions;
  };

  return (
    <Box component="section" sx={{ mt: 2 }}>
      <Typography variant="h6" component="h2" gutterBottom>
        {t('network.listView')}
      </Typography>
      <List dense>
        {contactNodes.map((contact) => {
          const descriptions = describeConnections(contact);
          // Issue #383/ADR-0023: the color dot alone is not an accessible
          // status indicator (WCAG 1.4.1, Use of Color) -- colorblind users
          // can't distinguish moss/chanterelle/russula by hue, so every dot
          // is paired with the same band name as visible text, not just an
          // aria-label. No band (old cached graph data, or a score that
          // hasn't been computed) renders neither.
          const dotColor = healthBandColor(contact.health_band, theme);
          const bandLabel = contact.health_band
            ? t(`contactScore.badge.${contact.health_band}`, contact.health_band)
            : undefined;
          return (
            <ListItem
              key={contact.id}
              disableGutters
              disablePadding
              sx={{ display: 'block', mb: 0.5 }}
            >
              <ListItemButton
                onClick={() => onContactClick(contact)}
                sx={{ display: 'inline-flex', alignItems: 'center', borderRadius: 1, py: 0.25 }}
              >
                {dotColor && (
                  // Inline `style` (not sx `bgcolor`) because dotColor is an
                  // arbitrary computed hex value, not a static theme token --
                  // this also makes the color directly assertable in tests
                  // via style.backgroundColor rather than an emotion class.
                  <Box
                    component="span"
                    aria-hidden="true"
                    style={{ backgroundColor: dotColor }}
                    sx={{
                      width: 8,
                      height: 8,
                      borderRadius: '50%',
                      mr: 0.75,
                      flexShrink: 0,
                    }}
                  />
                )}
                <ListItemText primary={contact.label} />
                {bandLabel && (
                  <Typography
                    variant="caption"
                    component="span"
                    sx={{ color: 'text.secondary', ml: 0.75 }}
                  >
                    ({bandLabel})
                  </Typography>
                )}
              </ListItemButton>
              {descriptions.length > 0 && (
                <Typography
                  variant="body2"
                  component="span"
                  sx={{
                    color: 'text.secondary',
                    ml: 1,
                  }}
                >
                  — {descriptions.join('; ')}
                </Typography>
              )}
            </ListItem>
          );
        })}
      </List>
    </Box>
  );
}
