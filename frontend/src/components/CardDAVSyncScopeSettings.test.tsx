import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import {
  type CardDAVSyncScope,
  getCardDAVSyncScope,
  previewCardDAVSyncScope,
  putCardDAVSyncScope,
} from '../api/carddavSyncScope';
import { type Circle, listCircles } from '../api/circles';
import { getHealth } from '../api/health';
import { SnackbarProvider } from '../context/SnackbarContext';
import CardDAVSyncScopeSettings from './CardDAVSyncScopeSettings';

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

vi.mock('../api/health', () => ({ getHealth: vi.fn() }));
vi.mock('../api/carddavSyncScope', () => ({
  getCardDAVSyncScope: vi.fn(),
  putCardDAVSyncScope: vi.fn(),
  previewCardDAVSyncScope: vi.fn(),
}));
vi.mock('../api/circles', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/circles')>();
  return { ...actual, listCircles: vi.fn() };
});

const getHealthMock = vi.mocked(getHealth);
const getScopeMock = vi.mocked(getCardDAVSyncScope);
const putScopeMock = vi.mocked(putCardDAVSyncScope);
const previewMock = vi.mocked(previewCardDAVSyncScope);
const listCirclesMock = vi.mocked(listCircles);

const circle = (id: string, name: string): Circle => ({
  id,
  name,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
});

const allScope: CardDAVSyncScope = {
  mode: 'all',
  circle_ids: [],
  default_circle_id: null,
  updated_at: null,
  in_scope_count: 3,
  total_count: 3,
  warnings: [],
};

function renderCard() {
  return render(
    <SnackbarProvider>
      <CardDAVSyncScopeSettings />
    </SnackbarProvider>,
  );
}

beforeEach(() => {
  getHealthMock.mockReset();
  getScopeMock.mockReset();
  putScopeMock.mockReset();
  previewMock.mockReset();
  listCirclesMock.mockReset();

  getHealthMock.mockResolvedValue({
    status: 'healthy',
    timestamp: '2026-01-01T00:00:00Z',
    database: { status: 'healthy', response_time_ms: 1 },
    version: '1.0.0',
    capabilities: ['contacts', 'carddav'],
  });
  getScopeMock.mockResolvedValue(allScope);
  listCirclesMock.mockResolvedValue({
    circles: [circle('c1', 'Family'), circle('c2', 'Work')],
    total: 2,
    next_cursor: '',
    limit: 100,
  });
  previewMock.mockResolvedValue({
    mode: 'all',
    circle_ids: [],
    in_scope_count: 3,
    total_count: 3,
    warnings: [],
  });
});

test('hidden when the deployment does not advertise CardDAV', async () => {
  getHealthMock.mockResolvedValue({
    status: 'healthy',
    timestamp: '2026-01-01T00:00:00Z',
    database: { status: 'healthy', response_time_ms: 1 },
    version: '1.0.0',
    capabilities: ['contacts'],
  });

  const { container } = renderCard();
  await waitFor(() => expect(container).toBeEmptyDOMElement());
  expect(getScopeMock).not.toHaveBeenCalled();
});

test('shows the all-contacts default and live counts', async () => {
  renderCard();
  expect(await screen.findByText('CardDAV sync scope')).toBeInTheDocument();
  expect(screen.getByText('3 of 3 contacts will sync.')).toBeInTheDocument();
});

test('switching to included circles saves the normalized scope', async () => {
  const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);
  previewMock.mockResolvedValue({
    mode: 'include',
    circle_ids: ['c1'],
    in_scope_count: 2,
    total_count: 3,
    warnings: [],
  });
  putScopeMock.mockResolvedValue({
    mode: 'include',
    circle_ids: ['c1'],
    default_circle_id: 'c1',
    updated_at: '2026-01-01T00:00:00Z',
    in_scope_count: 2,
    total_count: 3,
    warnings: [],
  });

  renderCard();
  await screen.findByText('CardDAV sync scope');

  fireEvent.mouseDown(screen.getByLabelText('Which contacts to sync'));
  fireEvent.click(await screen.findByRole('option', { name: 'Only selected circles' }));

  const circlesInput = await screen.findByLabelText('Included circles');
  fireEvent.change(circlesInput, { target: { value: 'Family' } });
  fireEvent.click(await screen.findByRole('option', { name: 'Family' }));

  fireEvent.click(screen.getByRole('button', { name: 'Save sync scope' }));

  await waitFor(() =>
    expect(putScopeMock).toHaveBeenCalledWith({
      mode: 'include',
      circle_ids: ['c1'],
      default_circle_id: 'c1',
    }),
  );
  expect(confirmSpy).toHaveBeenCalledWith(
    'Saving will remove 1 contact from your devices on the next sync. Mycorrhizal keeps the record. Continue?',
  );
  expect(await screen.findByText('CardDAV sync scope saved.')).toBeInTheDocument();
  confirmSpy.mockRestore();
});

test('declining the removal confirmation does not save', async () => {
  const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false);
  previewMock.mockResolvedValue({
    mode: 'include',
    circle_ids: ['c1'],
    in_scope_count: 2,
    total_count: 3,
    warnings: [],
  });

  renderCard();
  await screen.findByText('CardDAV sync scope');
  fireEvent.mouseDown(screen.getByLabelText('Which contacts to sync'));
  fireEvent.click(await screen.findByRole('option', { name: 'Only selected circles' }));
  const circlesInput = await screen.findByLabelText('Included circles');
  fireEvent.change(circlesInput, { target: { value: 'Family' } });
  fireEvent.click(await screen.findByRole('option', { name: 'Family' }));
  fireEvent.click(screen.getByRole('button', { name: 'Save sync scope' }));

  await waitFor(() => expect(confirmSpy).toHaveBeenCalled());
  expect(putScopeMock).not.toHaveBeenCalled();
  confirmSpy.mockRestore();
});

test('renders the derived warnings from the server', async () => {
  getScopeMock.mockResolvedValue({
    ...allScope,
    mode: 'include',
    warnings: [
      { code: 'scope_empty', message: 'ignored server English' },
      { code: 'default_circle_missing', message: 'ignored server English' },
    ],
  });
  renderCard();
  expect(
    await screen.findByText(
      'No circles are selected, so no contacts will sync to CardDAV clients.',
    ),
  ).toBeInTheDocument();
  expect(
    screen.getByText('No default circle is set, so contacts created on a phone cannot be saved.'),
  ).toBeInTheDocument();
});
