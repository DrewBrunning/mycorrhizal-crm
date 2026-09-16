import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import './i18n/config';
import type { Activity } from './api/activities';
import type { GraphData, GraphNode } from './types/graph';

// NetworkGraph renders to a canvas via react-force-graph-2d, which is out of
// scope here (triaged as low-value) and not meaningfully testable under
// jsdom. Replace it with a stub that exposes the callback props NetworkPage
// wires up, so this file can exercise NetworkPage's own handlers without
// touching canvas rendering.
vi.mock('./components/NetworkGraph', () => ({
  default: (props: {
    onNodeClick: (node: GraphNode) => void;
    onActivityClick?: (node: GraphNode) => void;
    selectedCircle?: string;
  }) => (
    <div data-testid="network-graph-stub">
      <div data-testid="selected-circle">{props.selectedCircle ?? ''}</div>
      <button
        type="button"
        onClick={() => props.onNodeClick({ id: 'c-1', type: 'contact', label: 'Alice' })}
      >
        trigger-node-click
      </button>
      <button
        type="button"
        onClick={() =>
          props.onActivityClick?.({ id: 'a-7', type: 'activity', label: 'Coffee Chat' })
        }
      >
        trigger-activity-click
      </button>
    </div>
  ),
}));

const mockUseGraphResult = {
  data: {
    nodes: [
      { id: 'c-1', type: 'contact', label: 'Alice' },
      { id: 'a-7', type: 'activity', label: 'Coffee Chat' },
    ],
    edges: [],
  } as GraphData,
  loading: false,
  error: null as string | null,
  refetch: vi.fn(),
};

vi.mock('./hooks/useGraph', () => ({
  useGraph: () => mockUseGraphResult,
}));

vi.mock('./hooks/useCircles', () => ({
  useCircles: () => ({
    circles: [
      { id: 'circle-1', name: 'Family', created_at: '', updated_at: '' },
      { id: 'circle-2', name: 'Work', created_at: '', updated_at: '' },
    ],
    circleNamesByUid: new Map<string, string[]>(),
  }),
}));

const mockActivity: Activity = {
  ID: 7,
  title: 'Coffee Chat',
  description: 'Catch up',
  location: 'Cafe',
  date: '2026-03-01T00:00:00Z',
  CreatedAt: '2026-01-01T00:00:00Z',
  UpdatedAt: '2026-01-01T00:00:00Z',
  contacts: [],
};

const getActivity = vi.fn();
const updateActivity = vi.fn();
const deleteActivity = vi.fn();

vi.mock('./api/activities', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/activities')>();
  return {
    ...actual,
    getActivity: (...args: unknown[]) => getActivity(...args),
    updateActivity: (...args: unknown[]) => updateActivity(...args),
    deleteActivity: (...args: unknown[]) => deleteActivity(...args),
  };
});

const getAllContacts = vi.fn();

vi.mock('./api/contacts', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/contacts')>();
  return {
    ...actual,
    getAllContacts: (...args: unknown[]) => getAllContacts(...args),
  };
});

beforeEach(() => {
  localStorage.setItem(
    'user_info',
    JSON.stringify({ user_id: 1, username: 'test', is_admin: false }),
  );
  getActivity.mockReset().mockResolvedValue(mockActivity);
  updateActivity.mockReset().mockResolvedValue({ ...mockActivity });
  deleteActivity.mockReset().mockResolvedValue(undefined);
  getAllContacts.mockReset().mockResolvedValue([]);
  mockUseGraphResult.error = null;
});

afterEach(() => {
  cleanup();
  localStorage.clear();
  vi.clearAllMocks();
});

async function renderPage() {
  const { default: NetworkPage } = await import('./NetworkPage');
  return render(
    <MemoryRouter>
      <NetworkPage />
    </MemoryRouter>,
  );
}

test('handleActivityNodeClick fetches the activity and opens the edit dialog', async () => {
  await renderPage();

  fireEvent.click(screen.getByRole('button', { name: 'trigger-activity-click' }));

  await waitFor(() => {
    expect(getActivity).toHaveBeenCalledWith(7);
  });
  expect(getAllContacts).toHaveBeenCalledWith({ limit: 1000 });

  await waitFor(() => {
    expect(screen.getByDisplayValue('Coffee Chat')).toBeDefined();
  });
});

test('handleActivityNodeClick logs and does not open the dialog when the fetch fails', async () => {
  const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});
  getActivity.mockRejectedValueOnce(new Error('not found'));

  await renderPage();
  fireEvent.click(screen.getByRole('button', { name: 'trigger-activity-click' }));

  await waitFor(() => {
    expect(consoleError).toHaveBeenCalledWith('Failed to fetch activity:', expect.any(Error));
  });
  expect(screen.queryByDisplayValue('Coffee Chat')).toBeNull();
  consoleError.mockRestore();
});

test('handleActivitySave submits the edited fields and closes the dialog', async () => {
  await renderPage();

  fireEvent.click(screen.getByRole('button', { name: 'trigger-activity-click' }));
  await waitFor(() => {
    expect(screen.getByDisplayValue('Coffee Chat')).toBeDefined();
  });

  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await waitFor(() => {
    expect(updateActivity).toHaveBeenCalledWith(
      7,
      expect.objectContaining({
        title: 'Coffee Chat',
        description: 'Catch up',
        location: 'Cafe',
        contact_ids: [],
      }),
    );
  });

  await waitFor(() => {
    expect(screen.queryByDisplayValue('Coffee Chat')).toBeNull();
  });
});

test('handleActivitySave does nothing when the title has been cleared', async () => {
  await renderPage();

  fireEvent.click(screen.getByRole('button', { name: 'trigger-activity-click' }));
  await waitFor(() => {
    expect(screen.getByDisplayValue('Coffee Chat')).toBeDefined();
  });

  const titleInput = screen.getByDisplayValue('Coffee Chat');
  fireEvent.change(titleInput, { target: { value: '   ' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  // Guard clause returns before calling updateActivity, and the dialog stays open.
  expect(updateActivity).not.toHaveBeenCalled();
  expect(screen.getByRole('button', { name: 'Save' })).toBeDefined();
});

test('handleActivitySave logs on a failed update and leaves the dialog open', async () => {
  const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});
  updateActivity.mockRejectedValueOnce(new Error('server down'));

  await renderPage();
  fireEvent.click(screen.getByRole('button', { name: 'trigger-activity-click' }));
  await waitFor(() => {
    expect(screen.getByDisplayValue('Coffee Chat')).toBeDefined();
  });

  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await waitFor(() => {
    expect(consoleError).toHaveBeenCalledWith('Failed to update activity:', expect.any(Error));
  });
  expect(screen.getByDisplayValue('Coffee Chat')).toBeDefined();
  consoleError.mockRestore();
});

test('deleting the activity confirms, calls deleteActivity, and closes the dialog', async () => {
  const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);
  await renderPage();

  fireEvent.click(screen.getByRole('button', { name: 'trigger-activity-click' }));
  await waitFor(() => {
    expect(screen.getByDisplayValue('Coffee Chat')).toBeDefined();
  });

  fireEvent.click(screen.getByRole('button', { name: 'Delete' }));

  await waitFor(() => {
    expect(deleteActivity).toHaveBeenCalledWith(7);
  });
  await waitFor(() => {
    expect(screen.queryByDisplayValue('Coffee Chat')).toBeNull();
  });
  confirmSpy.mockRestore();
});

test('handleCircleChange updates the selected circle passed down to the graph', async () => {
  await renderPage();

  fireEvent.mouseDown(screen.getByLabelText('Filter by Circle'));
  fireEvent.click(screen.getByRole('option', { name: 'Family' }));

  await waitFor(() => {
    expect(screen.getByTestId('selected-circle').textContent).toBe('Family');
  });
});

test('filter toggles read their initial state from localStorage', async () => {
  localStorage.setItem('network-show-relationships', 'false');
  localStorage.setItem('network-show-activities', 'false');
  localStorage.setItem('network-show-circles', 'true');

  await renderPage();

  expect((screen.getByLabelText('Relationships') as HTMLInputElement).checked).toBe(false);
  expect((screen.getByLabelText('Activities') as HTMLInputElement).checked).toBe(false);
  expect((screen.getByLabelText('Circles') as HTMLInputElement).checked).toBe(true);
});

test('toggling a filter switch persists the new value to localStorage', async () => {
  await renderPage();

  // Defaults: relationships/activities on, circles off.
  expect(localStorage.getItem('network-show-circles')).toBe('false');

  fireEvent.click(screen.getByLabelText('Circles'));

  await waitFor(() => {
    expect(localStorage.getItem('network-show-circles')).toBe('true');
  });

  fireEvent.click(screen.getByLabelText('Relationships'));
  await waitFor(() => {
    expect(localStorage.getItem('network-show-relationships')).toBe('false');
  });
});
