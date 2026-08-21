import { test, expect, vi, afterEach, beforeEach } from 'vitest';
import { renderHook, cleanup, waitFor, act } from '@testing-library/react';
import { useRelationshipEdges } from './useRelationshipEdges';
import {
  getRelationshipEdges,
  createRelationshipEdge,
  updateRelationshipEdge,
  deleteRelationshipEdge,
  acceptRelationshipEdge,
  rejectRelationshipEdge,
  RelationshipEdge,
  RelationshipEdgeInput,
  RelationshipEdgesResponse,
} from '../api/relationshipEdges';
import { getContactsByUid, Contact } from '../api/contacts';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

vi.mock('../api/relationshipEdges', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/relationshipEdges')>();
  return {
    ...actual,
    getRelationshipEdges: vi.fn(),
    createRelationshipEdge: vi.fn(),
    updateRelationshipEdge: vi.fn(),
    deleteRelationshipEdge: vi.fn(),
    acceptRelationshipEdge: vi.fn(),
    rejectRelationshipEdge: vi.fn(),
  };
});

vi.mock('../api/contacts', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/contacts')>();
  return { ...actual, getContactsByUid: vi.fn() };
});

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(getRelationshipEdges).mockReset();
  vi.mocked(createRelationshipEdge).mockReset();
  vi.mocked(updateRelationshipEdge).mockReset();
  vi.mocked(deleteRelationshipEdge).mockReset();
  vi.mocked(acceptRelationshipEdge).mockReset();
  vi.mocked(rejectRelationshipEdge).mockReset();
  vi.mocked(getContactsByUid).mockReset();
});

const otherContact: Contact = { ID: 2, firstname: 'Bob', lastname: 'Burns', uid: 'bob-uid' };

function edge(id: string, overrides: Partial<RelationshipEdge> = {}): RelationshipEdge {
  return {
    id,
    source_id: 'alice-uid',
    target_id: 'bob-uid',
    type: 'parent_of',
    directional: true,
    source: 'user-confirmed',
    confidence: 1,
    status: 'confirmed',
    sensitivity: 'normal',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

function listResponse(edges: RelationshipEdge[]): RelationshipEdgesResponse {
  return { relationship_edges: edges, total: edges.length, next_cursor: '', limit: 100 };
}

test('loads edges and resolves the other parties', async () => {
  vi.mocked(getRelationshipEdges).mockResolvedValue(listResponse([edge('e-1')]));
  vi.mocked(getContactsByUid).mockResolvedValue(new Map([['bob-uid', otherContact]]));

  const { result } = renderHook(() => useRelationshipEdges('alice-uid'));
  await act(async () => {
    await result.current.refreshRelationshipEdges();
  });

  expect(getRelationshipEdges).toHaveBeenCalledWith({ contactId: 'alice-uid', limit: 100 });
  expect(getContactsByUid).toHaveBeenCalledWith(['bob-uid']);
  expect(result.current.edges).toHaveLength(1);
  expect(result.current.contactsByUid.get('bob-uid')).toEqual(otherContact);
  expect(result.current.error).toBeNull();
});

test('splits confirmed and suggested edges', async () => {
  vi.mocked(getRelationshipEdges).mockResolvedValue(
    listResponse([
      edge('e-1'),
      edge('e-2', { status: 'suggested', source: 'household-inferred' }),
    ])
  );
  vi.mocked(getContactsByUid).mockResolvedValue(new Map());

  const { result } = renderHook(() => useRelationshipEdges('alice-uid'));
  await act(async () => {
    await result.current.refreshRelationshipEdges();
  });

  expect(result.current.confirmedEdges.map((e) => e.id)).toEqual(['e-1']);
  expect(result.current.suggestedEdges.map((e) => e.id)).toEqual(['e-2']);
});

test('does not fetch without a viewed contact uid', async () => {
  const { result } = renderHook(() => useRelationshipEdges(undefined));
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(getRelationshipEdges).not.toHaveBeenCalled();
  expect(result.current.edges).toEqual([]);
});

test('handleAddRelationshipEdge opens the dialog for a new edge', () => {
  const { result } = renderHook(() => useRelationshipEdges('alice-uid'));
  act(() => result.current.handleAddRelationshipEdge());
  expect(result.current.relationshipDialogOpen).toBe(true);
  expect(result.current.editingEdge).toBeNull();
});

test('handleEditRelationshipEdge loads the edge into the dialog', async () => {
  vi.mocked(getRelationshipEdges).mockResolvedValue(listResponse([edge('e-1')]));
  vi.mocked(getContactsByUid).mockResolvedValue(new Map());

  const { result } = renderHook(() => useRelationshipEdges('alice-uid'));
  await act(async () => {
    await result.current.refreshRelationshipEdges();
  });

  act(() => result.current.handleEditRelationshipEdge(result.current.edges[0]));
  expect(result.current.editingEdge?.id).toBe('e-1');
  expect(result.current.relationshipDialogOpen).toBe(true);
});

test('handleSaveRelationshipEdge creates when nothing is being edited', async () => {
  vi.mocked(getRelationshipEdges).mockResolvedValue(listResponse([]));
  vi.mocked(getContactsByUid).mockResolvedValue(new Map());
  vi.mocked(createRelationshipEdge).mockResolvedValue(edge('e-9'));

  const { result } = renderHook(() => useRelationshipEdges('alice-uid'));
  await act(async () => {
    await result.current.refreshRelationshipEdges();
  });

  const input: RelationshipEdgeInput = { target_id: 'bob-uid', type: 'friend_of' };
  await act(async () => {
    await result.current.handleSaveRelationshipEdge(input);
  });

  expect(createRelationshipEdge).toHaveBeenCalledWith(input);
  expect(updateRelationshipEdge).not.toHaveBeenCalled();
  expect(result.current.relationshipDialogOpen).toBe(false);
});

test('handleSaveRelationshipEdge updates when editing an existing edge', async () => {
  vi.mocked(getRelationshipEdges).mockResolvedValue(listResponse([edge('e-1')]));
  vi.mocked(getContactsByUid).mockResolvedValue(new Map());

  const { result } = renderHook(() => useRelationshipEdges('alice-uid'));
  await act(async () => {
    await result.current.refreshRelationshipEdges();
  });

  act(() => result.current.handleEditRelationshipEdge(result.current.edges[0]));

  const input: RelationshipEdgeInput = { source_id: 'alice-uid', target_id: 'bob-uid', type: 'spouse_of' };
  await act(async () => {
    await result.current.handleSaveRelationshipEdge(input);
  });

  expect(updateRelationshipEdge).toHaveBeenCalledWith('e-1', input);
  expect(createRelationshipEdge).not.toHaveBeenCalled();
  expect(result.current.editingEdge).toBeNull();
  expect(result.current.relationshipDialogOpen).toBe(false);
});

test('handleDeleteRelationshipEdge deletes and refreshes', async () => {
  vi.mocked(getRelationshipEdges).mockResolvedValue(listResponse([edge('e-1')]));
  vi.mocked(getContactsByUid).mockResolvedValue(new Map());

  const { result } = renderHook(() => useRelationshipEdges('alice-uid'));
  await act(async () => {
    await result.current.refreshRelationshipEdges();
  });

  await act(async () => {
    await result.current.handleDeleteRelationshipEdge('e-1');
  });

  expect(deleteRelationshipEdge).toHaveBeenCalledWith('e-1');
  expect(getRelationshipEdges).toHaveBeenCalledTimes(2);
});

test('handleAcceptSuggestion accepts and refreshes', async () => {
  vi.mocked(getRelationshipEdges).mockResolvedValue(listResponse([]));
  vi.mocked(getContactsByUid).mockResolvedValue(new Map());

  const { result } = renderHook(() => useRelationshipEdges('alice-uid'));
  await act(async () => {
    await result.current.refreshRelationshipEdges();
  });

  await act(async () => {
    await result.current.handleAcceptSuggestion('e-1');
  });

  expect(acceptRelationshipEdge).toHaveBeenCalledWith('e-1');
  expect(getRelationshipEdges).toHaveBeenCalledTimes(2);
});

test('handleRejectSuggestion rejects and refreshes', async () => {
  vi.mocked(getRelationshipEdges).mockResolvedValue(listResponse([]));
  vi.mocked(getContactsByUid).mockResolvedValue(new Map());

  const { result } = renderHook(() => useRelationshipEdges('alice-uid'));
  await act(async () => {
    await result.current.refreshRelationshipEdges();
  });

  await act(async () => {
    await result.current.handleRejectSuggestion('e-1');
  });

  expect(rejectRelationshipEdge).toHaveBeenCalledWith('e-1');
  expect(getRelationshipEdges).toHaveBeenCalledTimes(2);
});

test('save errors notify through the notifier and rethrow', async () => {
  vi.mocked(getRelationshipEdges).mockResolvedValue(listResponse([]));
  vi.mocked(getContactsByUid).mockResolvedValue(new Map());
  vi.mocked(createRelationshipEdge).mockRejectedValue(new Error('boom'));
  const showError = vi.fn();

  const { result } = renderHook(() => useRelationshipEdges('alice-uid', { showError }));
  await waitFor(() => expect(result.current.loading).toBe(false));

  await expect(
    result.current.handleSaveRelationshipEdge({ target_id: 'bob-uid', type: 'friend_of' })
  ).rejects.toThrow('boom');
  expect(showError).toHaveBeenCalledWith('boom');
});

test('sets error when the fetch fails', async () => {
  vi.mocked(getRelationshipEdges).mockRejectedValue(new Error('boom'));

  const { result } = renderHook(() => useRelationshipEdges('alice-uid'));
  await act(async () => {
    await result.current.refreshRelationshipEdges();
  });

  expect(result.current.error).toBe('boom');
  expect(result.current.edges).toEqual([]);
});
