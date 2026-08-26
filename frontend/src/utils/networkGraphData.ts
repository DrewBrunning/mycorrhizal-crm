// Shared node/edge filtering for the network graph.
//
// Pulled out of NetworkGraph.tsx (T189) so NetworkPage's accessible list view
// can render exactly the same filtered set the canvas draws, from the same
// pure function and the same inputs. Two independent re-implementations of
// this filtering would be free to drift out of sync silently; a single
// shared function cannot.
import type { GraphData, GraphEdge, GraphNode } from '../types/graph';

export interface NetworkGraphFilters {
  selectedCircle?: string;
  showRelationships: boolean;
  showActivities: boolean;
  showCircles: boolean;
  centeredNodeId?: string;
  circleNamesByUid?: Map<string, string[]>;
}

export interface FilteredGraphData {
  nodes: GraphNode[];
  links: GraphEdge[];
}

export function edgeEndpointId(end: string | GraphNode): string {
  return typeof end === 'string' ? end : end.id;
}

export function computeFilteredGraphData(
  data: GraphData,
  {
    selectedCircle,
    showRelationships,
    showActivities,
    showCircles,
    centeredNodeId,
    circleNamesByUid,
  }: NetworkGraphFilters,
): FilteredGraphData {
  let filteredNodes = data.nodes;

  // Filter by circle if selected
  if (selectedCircle) {
    const contactsInCircle = new Set(
      data.nodes
        .filter((n) => {
          if (n.type !== 'contact' || !circleNamesByUid) return false;
          const contactId = n.id.replace('c-', '');
          return (circleNamesByUid.get(contactId) || []).includes(selectedCircle);
        })
        .map((n) => n.id),
    );

    // Include contacts in circle and activities that have at least 2 contacts in the circle
    filteredNodes = data.nodes.filter((n) => {
      if (n.type === 'contact') {
        return contactsInCircle.has(n.id);
      }
      // For activities, check if they connect contacts in this circle
      const activityEdges = data.edges.filter(
        (e) => e.type === 'activity' && edgeEndpointId(e.source) === n.id,
      );
      const connectedContacts = activityEdges.filter((e) =>
        contactsInCircle.has(edgeEndpointId(e.target)),
      );
      return connectedContacts.length >= 2;
    });
  }

  // Hide activity nodes when the activities toggle is off
  if (!showActivities) {
    filteredNodes = filteredNodes.filter((n) => n.type !== 'activity');
  }

  const nodeIds = new Set(filteredNodes.map((n) => n.id));

  // Filter edges based on visibility toggles and filtered nodes
  let filteredEdges = data.edges.filter((e) => {
    const sourceId = edgeEndpointId(e.source);
    const targetId = edgeEndpointId(e.target);

    if (!nodeIds.has(sourceId) || !nodeIds.has(targetId)) return false;
    if (e.type === 'relationship' && !showRelationships) return false;
    return true;
  });

  // Synthesize circle nodes and edges from contact circle memberships
  if (showCircles && circleNamesByUid) {
    const visibleContacts = filteredNodes.filter((n) => n.type === 'contact');

    // Count contacts per circle
    const circleContactMap = new Map<string, string[]>();
    visibleContacts.forEach((contact) => {
      const contactId = contact.id.replace('c-', '');
      const names = circleNamesByUid.get(contactId) || [];
      names.forEach((circleName) => {
        const existing = circleContactMap.get(circleName) ?? [];
        existing.push(contact.id);
        circleContactMap.set(circleName, existing);
      });
    });

    const circleNodes: GraphNode[] = [];
    const circleEdges: GraphEdge[] = [];

    circleContactMap.forEach((contactIds, circleName) => {
      if (contactIds.length < 2) return; // only show circles that connect people

      const circleNodeId = `circle-${circleName}`;
      circleNodes.push({
        id: circleNodeId,
        type: 'circle',
        label: circleName,
      });

      contactIds.forEach((contactId) => {
        circleEdges.push({
          id: `ce-${contactId}-${circleName}`,
          type: 'circle',
          source: contactId,
          target: circleNodeId,
          label: circleName,
        });
      });
    });

    filteredNodes = [...filteredNodes, ...circleNodes];
    filteredEdges = [...filteredEdges, ...circleEdges];
  }

  if (centeredNodeId) {
    const directNeighbors = new Set<string>([centeredNodeId]);

    filteredEdges.forEach((e) => {
      const srcId = edgeEndpointId(e.source);
      const tgtId = edgeEndpointId(e.target);
      if (srcId === centeredNodeId) directNeighbors.add(tgtId);
      if (tgtId === centeredNodeId) directNeighbors.add(srcId);
    });

    filteredNodes = filteredNodes.filter((n) => directNeighbors.has(n.id));
    filteredEdges = filteredEdges.filter((e) => {
      const srcId = edgeEndpointId(e.source);
      const tgtId = edgeEndpointId(e.target);
      return directNeighbors.has(srcId) && directNeighbors.has(tgtId);
    });
  }

  return {
    nodes: filteredNodes,
    links: filteredEdges,
  };
}
