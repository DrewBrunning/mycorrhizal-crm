import AddIcon from '@mui/icons-material/Add';
import CenterFocusStrongIcon from '@mui/icons-material/CenterFocusStrong';
import KeyboardArrowDownIcon from '@mui/icons-material/KeyboardArrowDown';
import KeyboardArrowLeftIcon from '@mui/icons-material/KeyboardArrowLeft';
import KeyboardArrowRightIcon from '@mui/icons-material/KeyboardArrowRight';
import KeyboardArrowUpIcon from '@mui/icons-material/KeyboardArrowUp';
import RemoveIcon from '@mui/icons-material/Remove';
import { Box, IconButton, Typography, useMediaQuery, useTheme } from '@mui/material';
import { forceX, forceY } from 'd3-force';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import ForceGraph2D from 'react-force-graph-2d';
import { useTranslation } from 'react-i18next';
import type { GraphData, GraphEdge, GraphNode } from '../types/graph';
import { computeFilteredGraphData } from '../utils/networkGraphData';

interface NetworkGraphProps {
  data: GraphData;
  onNodeClick: (node: GraphNode) => void;
  onActivityClick?: (node: GraphNode) => void;
  selectedCircle?: string;
  showRelationships: boolean;
  showActivities: boolean;
  showCircles: boolean;
  centeredNodeId?: string;
  circleNamesByUid?: Map<string, string[]>;
}

interface ForceGraphData {
  nodes: GraphNode[];
  links: GraphEdge[];
}

const getNodeSize = (type: GraphNode['type']): number => {
  if (type === 'contact') return 12;
  if (type === 'circle') return 9;
  return 6;
};

export default function NetworkGraph({
  data,
  onNodeClick,
  onActivityClick,
  selectedCircle,
  showRelationships,
  showActivities,
  showCircles,
  centeredNodeId,
  circleNamesByUid,
}: NetworkGraphProps) {
  const { t } = useTranslation();
  const theme = useTheme();
  const containerRef = useRef<HTMLDivElement>(null);
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const graphRef = useRef<any>(null);
  const [dimensions, setDimensions] = useState({ width: 800, height: 600 });
  const [hoveredEdge, setHoveredEdge] = useState<GraphEdge | null>(null);
  const [hoveredNode, setHoveredNode] = useState<GraphNode | null>(null);
  const [tooltipPos, setTooltipPos] = useState({ x: 0, y: 0 });
  const isMobile = useMediaQuery(theme.breakpoints.down('md'));
  // #194: under reduced motion, settle the force simulation instantly and
  // skip the initial zoom-to-fit animation instead of running them
  // regardless of the OS setting (WCAG 2.3.3, AAA).
  const prefersReducedMotion = useMediaQuery('(prefers-reduced-motion: reduce)');

  // Colors from theme
  const relationshipColor = theme.palette.primary.main;
  const activityColor = theme.palette.secondary.main;
  const circleEdgeColor = theme.palette.warning.main;
  const nodeColor = theme.palette.primary.main;
  const activityNodeColor = theme.palette.secondary.main;
  const circleNodeColor = theme.palette.warning.main;
  const textColor = theme.palette.text.primary;
  const bgColor = theme.palette.background.paper;

  // Handle container resize and mouse tracking
  useEffect(() => {
    const updateDimensions = () => {
      if (containerRef.current) {
        const { width, height } = containerRef.current.getBoundingClientRect();
        setDimensions({ width, height });
      }
    };

    const handleMouseMove = (e: MouseEvent) => {
      setTooltipPos({ x: e.clientX, y: e.clientY });
    };

    updateDimensions();
    window.addEventListener('resize', updateDimensions);
    window.addEventListener('mousemove', handleMouseMove);
    return () => {
      window.removeEventListener('resize', updateDimensions);
      window.removeEventListener('mousemove', handleMouseMove);
    };
  }, []);

  // Filter and transform data for the graph, including synthetic circle
  // nodes/edges. Delegates to the shared computeFilteredGraphData so
  // NetworkPage's accessible list view (T189) filters identically -- same
  // pure function, same inputs, so the two can never disagree.
  const graphData: ForceGraphData = useMemo(
    () =>
      computeFilteredGraphData(data, {
        selectedCircle,
        showRelationships,
        showActivities,
        showCircles,
        centeredNodeId,
        circleNamesByUid,
      }),
    [
      data,
      selectedCircle,
      showRelationships,
      showActivities,
      showCircles,
      centeredNodeId,
      circleNamesByUid,
    ],
  );

  // Center and zoom to selected node when centeredNodeId changes
  useEffect(() => {
    if (!centeredNodeId || !graphRef.current) return;

    const node = graphData.nodes.find((n) => n.id === centeredNodeId);
    if (!node || node.x == null || node.y == null) return;

    graphRef.current.centerAt(node.x, node.y, 800);
    graphRef.current.zoom(2.5, 800);
  }, [centeredNodeId, graphData.nodes]);

  // Get initials from a name
  const getInitials = (label: string): string => {
    const parts = label.split(' ').filter(Boolean);
    if (parts.length >= 2) {
      return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase();
    }
    return label.substring(0, 2).toUpperCase();
  };

  // Custom node rendering
  const nodeCanvasObject = useCallback(
    (node: GraphNode, ctx: CanvasRenderingContext2D, globalScale: number) => {
      const isContact = node.type === 'contact';
      const isActivity = node.type === 'activity';
      const isCircleNode = node.type === 'circle';
      const size = getNodeSize(node.type);
      const fontSize = Math.max(10 / globalScale, 3);
      const isCentered = node.id === centeredNodeId;

      // Draw highlight ring for centered contact
      if (isCentered) {
        ctx.beginPath();
        ctx.arc(node.x || 0, node.y || 0, size + 5, 0, 2 * Math.PI);
        ctx.strokeStyle = theme.palette.primary.light;
        ctx.lineWidth = 3 / globalScale;
        ctx.stroke();
      }

      // Draw node circle
      ctx.beginPath();
      ctx.arc(node.x || 0, node.y || 0, size, 0, 2 * Math.PI);
      if (isContact) {
        ctx.fillStyle = nodeColor;
      } else if (isActivity) {
        ctx.fillStyle = activityNodeColor;
      } else {
        ctx.fillStyle = circleNodeColor;
      }
      ctx.fill();

      // Draw border
      ctx.strokeStyle = bgColor;
      ctx.lineWidth = 2 / globalScale;
      ctx.stroke();

      // Draw initials for contacts
      if (isContact && globalScale > 0.5) {
        ctx.font = `bold ${fontSize * 1.2}px Helvetica, Helvetica Neue, Roboto, Arial, sans-serif`;

        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';
        ctx.fillStyle = '#FFFFFF';
        ctx.fillText(getInitials(node.label), node.x || 0, node.y || 0);
      }

      // Draw label below node when zoomed in enough
      if (globalScale > 0.6 && (isContact || isActivity || isCircleNode)) {
        ctx.font = `${fontSize}px Helvetica, Helvetica Neue, Roboto, Arial, sans-serif`;
        ctx.textAlign = 'center';
        ctx.textBaseline = 'top';
        ctx.fillStyle = textColor;
        ctx.fillText(node.label, node.x || 0, (node.y || 0) + size + 4);
      }
    },
    [
      nodeColor,
      activityNodeColor,
      circleNodeColor,
      bgColor,
      textColor,
      centeredNodeId,
      theme.palette.primary.light,
    ],
  );

  // Custom link rendering
  const linkColor = useCallback(
    (link: GraphEdge) => {
      if (link.type === 'relationship') return relationshipColor;
      if (link.type === 'activity') return activityColor;
      return circleEdgeColor;
    },
    [relationshipColor, activityColor, circleEdgeColor],
  );

  // Handle node hover
  const handleNodeHover = useCallback((node: GraphNode | null) => {
    setHoveredNode(node);
  }, []);

  // Handle link hover
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const handleLinkHover = useCallback((link: any) => {
    setHoveredEdge(link as GraphEdge | null);
  }, []);

  // Handle node click
  const handleNodeClick = useCallback(
    (node: GraphNode) => {
      if (node.type === 'contact') {
        onNodeClick(node);
      } else if (node.type === 'activity') {
        onActivityClick?.(node);
      }
    },
    [onNodeClick, onActivityClick],
  );

  // Configure forces to prevent isolated nodes from drifting too far
  useEffect(() => {
    if (graphRef.current) {
      const fg = graphRef.current;
      fg.d3Force('x', forceX(0).strength(0.05));
      fg.d3Force('y', forceY(0).strength(0.05));
      fg.d3Force('charge')?.strength(-100);
    }
  }, []);

  // Zoom to fit on initial load
  useEffect(() => {
    if (graphRef.current && graphData.nodes.length > 0) {
      setTimeout(() => {
        graphRef.current?.zoomToFit(prefersReducedMotion ? 0 : 400, isMobile ? 50 : 80);
      }, 500);
    }
  }, [graphData.nodes.length, isMobile, prefersReducedMotion]);

  // Non-drag alternatives for pan/zoom (#190, WCAG 2.5.7 Dragging Movements).
  // enableNodeDrag is left drag-only and deliberately has no button
  // equivalent: it only repositions a node within the force simulation for
  // the current session, nothing reads or persists node.x/node.y anywhere in
  // this codebase (confirmed via grep across api/ and hooks/), so there is no
  // state a keyboard/single-pointer user would be locked out of reaching.
  const handleZoomIn = useCallback(() => {
    const fg = graphRef.current;
    if (!fg) return;
    fg.zoom(fg.zoom() * 1.3, 250);
  }, []);

  const handleZoomOut = useCallback(() => {
    const fg = graphRef.current;
    if (!fg) return;
    fg.zoom(fg.zoom() / 1.3, 250);
  }, []);

  const handleResetView = useCallback(() => {
    graphRef.current?.zoomToFit(400, isMobile ? 50 : 80);
  }, [isMobile]);

  const handlePan = useCallback((dx: number, dy: number) => {
    const fg = graphRef.current;
    if (!fg) return;
    const { x, y } = fg.centerAt();
    // Step scales inversely with zoom so a pan press moves roughly the same
    // distance on screen regardless of how zoomed in the view currently is.
    const step = 120 / fg.zoom();
    fg.centerAt(x + dx * step, y + dy * step, 250);
  }, []);

  const getEdgeTypeLabel = (type: string) => {
    if (type === 'relationship') return t('network.legend.relationships');
    if (type === 'activity') return t('network.legend.activities');
    return t('network.legend.circleEdge');
  };

  // The canvas paints straight to pixels with no DOM text content -- role="img"
  // plus a localised, data-driven aria-label makes it a text alternative
  // (1.1.1) instead of leaving it invisible to assistive tech. The full data
  // is also available, always in the DOM, in NetworkPage's list view below
  // the graph, so the tooltip and canvas interactions stay pointer-only by
  // design (see #189) -- do not add tabindex here.
  //
  // Scoped to a Box that wraps ONLY the canvas, not the pan/zoom controls
  // (#190): the WAI-ARIA spec explicitly says a role="img" element's
  // descendants aren't guaranteed to be exposed to assistive tech ("authors
  // SHOULD NOT expect user agents to expose descendants"), so real
  // interactive controls must stay siblings of the img-role node, never
  // nested inside it, or a screen reader user could lose the buttons this
  // ticket exists to add.
  const graphSummary = t('network.graphSummary', {
    contacts: graphData.nodes.filter((n) => n.type === 'contact').length,
    activities: graphData.nodes.filter((n) => n.type === 'activity').length,
    circles: graphData.nodes.filter((n) => n.type === 'circle').length,
    connections: graphData.links.length,
  });

  return (
    <Box ref={containerRef} sx={{ width: '100%', height: '100%', position: 'relative' }}>
      <Box role="img" aria-label={graphSummary}>
        <ForceGraph2D
          ref={graphRef}
          width={dimensions.width}
          height={dimensions.height}
          graphData={graphData}
          nodeCanvasObject={nodeCanvasObject}
          nodePointerAreaPaint={(node: GraphNode, color, ctx) => {
            const size = getNodeSize(node.type);
            ctx.beginPath();
            ctx.arc(node.x || 0, node.y || 0, size + 4, 0, 2 * Math.PI);
            ctx.fillStyle = color;
            ctx.fill();
          }}
          linkColor={linkColor}
          linkWidth={2}
          linkDirectionalArrowLength={0}
          onNodeClick={handleNodeClick}
          onNodeHover={handleNodeHover}
          onLinkHover={handleLinkHover}
          cooldownTicks={prefersReducedMotion ? 0 : 100}
          // Cosmetic-only, session-local repositioning -- see the handlePan/
          // handleZoomIn comment above for why this has no button equivalent.
          enableNodeDrag={true}
          enableZoomInteraction={true}
          enablePanInteraction={true}
          backgroundColor={bgColor}
          nodeId="id"
          linkSource="source"
          linkTarget="target"
        />
      </Box>

      {/* Node / edge tooltip */}
      {(hoveredNode || hoveredEdge) && (
        <Box
          sx={{
            position: 'fixed',
            left: tooltipPos.x + 10,
            top: tooltipPos.y + 10,
            bgcolor: 'background.paper',
            border: 1,
            borderColor: 'divider',
            borderRadius: 1,
            px: 1.5,
            py: 0.75,
            boxShadow: 2,
            pointerEvents: 'none',
            zIndex: 1000,
          }}
        >
          {hoveredNode ? (
            <>
              <Typography variant="body2" sx={{ fontWeight: 500 }}>
                {hoveredNode.label}
              </Typography>
              <Typography variant="caption" color="text.secondary">
                {hoveredNode.type === 'contact'
                  ? t('network.legend.contact')
                  : hoveredNode.type === 'activity'
                    ? t('network.legend.activity')
                    : t('network.legend.circle')}
              </Typography>
            </>
          ) : hoveredEdge ? (
            <>
              <Typography variant="body2" sx={{ fontWeight: 500 }}>
                {hoveredEdge.label}
              </Typography>
              <Typography variant="caption" color="text.secondary">
                {getEdgeTypeLabel(hoveredEdge.type)}
              </Typography>
            </>
          ) : null}
        </Box>
      )}

      {/* Non-drag pan/zoom controls (#190) -- every transform reachable by
          drag or wheel is also reachable by a single click/keyboard press. */}
      <Box
        sx={{
          position: 'absolute',
          bottom: 8,
          right: 8,
          zIndex: 10,
          bgcolor: 'background.paper',
          borderRadius: 1,
          boxShadow: 2,
          p: 0.5,
        }}
      >
        <Box
          sx={{
            display: 'grid',
            gridTemplateColumns: 'repeat(3, 1fr)',
            gridTemplateRows: 'repeat(3, 1fr)',
            width: 96,
            height: 96,
          }}
        >
          <IconButton
            size="small"
            aria-label={t('network.panUp')}
            onClick={() => handlePan(0, -1)}
            sx={{ gridColumn: 2, gridRow: 1, minWidth: 24, minHeight: 24 }}
          >
            <KeyboardArrowUpIcon fontSize="small" />
          </IconButton>
          <IconButton
            size="small"
            aria-label={t('network.panLeft')}
            onClick={() => handlePan(-1, 0)}
            sx={{ gridColumn: 1, gridRow: 2, minWidth: 24, minHeight: 24 }}
          >
            <KeyboardArrowLeftIcon fontSize="small" />
          </IconButton>
          <IconButton
            size="small"
            aria-label={t('network.resetView')}
            onClick={handleResetView}
            sx={{ gridColumn: 2, gridRow: 2, minWidth: 24, minHeight: 24 }}
          >
            <CenterFocusStrongIcon fontSize="small" />
          </IconButton>
          <IconButton
            size="small"
            aria-label={t('network.panRight')}
            onClick={() => handlePan(1, 0)}
            sx={{ gridColumn: 3, gridRow: 2, minWidth: 24, minHeight: 24 }}
          >
            <KeyboardArrowRightIcon fontSize="small" />
          </IconButton>
          <IconButton
            size="small"
            aria-label={t('network.panDown')}
            onClick={() => handlePan(0, 1)}
            sx={{ gridColumn: 2, gridRow: 3, minWidth: 24, minHeight: 24 }}
          >
            <KeyboardArrowDownIcon fontSize="small" />
          </IconButton>
        </Box>
        <Box sx={{ display: 'flex', justifyContent: 'center', gap: 0.5, mt: 0.5 }}>
          <IconButton
            size="small"
            aria-label={t('network.zoomIn')}
            onClick={handleZoomIn}
            sx={{ minWidth: 24, minHeight: 24 }}
          >
            <AddIcon fontSize="small" />
          </IconButton>
          <IconButton
            size="small"
            aria-label={t('network.zoomOut')}
            onClick={handleZoomOut}
            sx={{ minWidth: 24, minHeight: 24 }}
          >
            <RemoveIcon fontSize="small" />
          </IconButton>
        </Box>
      </Box>
    </Box>
  );
}
