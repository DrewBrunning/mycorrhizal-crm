// Types for the network graph visualization

export interface GraphNode {
  id: string; // "c-{contactID}" for contacts, "a-{activityID}" for activities, "circle-{name}" for circles
  type: 'contact' | 'activity' | 'circle';
  label: string; // Contact name, activity title, or circle name
  photo_thumbnail?: string;
  // Relationship health score (issue #383, ADR-0023) -- set only on
  // type: "contact" nodes, never on "activity" or "circle" nodes. See
  // utils/healthBand.ts for the band -> color mapping.
  health_score?: number | null;
  health_band?: string;
  // Issue #1193: set only on type: "contact" nodes with a recorded death
  // anniversary (Card.Anniversaries[kind=death]). Takes precedence over
  // health_band for coloring/labeling -- a deceased contact's health score
  // (recency of interaction) is meaningless.
  deceased?: boolean;
  // Properties added by force-graph during rendering
  x?: number;
  y?: number;
  vx?: number;
  vy?: number;
}

export interface GraphEdge {
  id: string;
  source: string | GraphNode; // Can be ID string or resolved node object
  target: string | GraphNode;
  type: 'relationship' | 'activity' | 'circle';
  label: string;
}

export interface GraphData {
  nodes: GraphNode[];
  edges: GraphEdge[];
}

// Response from the API
export interface GraphResponse {
  nodes: GraphNode[];
  edges: GraphEdge[];
}
