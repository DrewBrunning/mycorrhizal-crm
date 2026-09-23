import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import * as lifeEventsApi from '../api/lifeEvents';
import LifeEventSuggestions from './LifeEventSuggestions';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const suggestion = {
  entity_id: 'contact-1',
  type: 'moved',
  category: 'home_living',
  date: { year: 2019 },
  end_date: { year: 2024 },
  source_kind: 'address',
  source_entry_id: 'addr-1',
};

test('renders nothing when there are no suggestions', async () => {
  vi.spyOn(lifeEventsApi, 'getLifeEventSuggestions').mockResolvedValue([]);
  render(<LifeEventSuggestions contactId={1} onAccepted={vi.fn()} />);

  await waitFor(() => expect(lifeEventsApi.getLifeEventSuggestions).toHaveBeenCalledWith(1));
  expect(screen.queryByText('Suggested life events')).not.toBeInTheDocument();
});

test('renders a prompt describing the inferred event and its period', async () => {
  vi.spyOn(lifeEventsApi, 'getLifeEventSuggestions').mockResolvedValue([suggestion]);
  render(<LifeEventSuggestions contactId={1} onAccepted={vi.fn()} />);

  expect(await screen.findByText('Suggested life events')).toBeInTheDocument();
  expect(screen.getByText(/Moved/)).toBeInTheDocument();
  expect(screen.getByText(/2019 – 2024/)).toBeInTheDocument();
});

test('dismiss records the rejection and removes the row', async () => {
  vi.spyOn(lifeEventsApi, 'getLifeEventSuggestions').mockResolvedValue([suggestion]);
  const resolve = vi
    .spyOn(lifeEventsApi, 'resolveLifeEventSuggestion')
    .mockResolvedValue(undefined);
  render(<LifeEventSuggestions contactId={1} onAccepted={vi.fn()} />);

  fireEvent.click(await screen.findByTestId('suggestion-dismiss'));

  await waitFor(() =>
    expect(resolve).toHaveBeenCalledWith({
      entity_id: 'contact-1',
      source_kind: 'address',
      source_entry_id: 'addr-1',
      event_type: 'moved',
      resolution: 'dismissed',
    }),
  );
  await waitFor(() => expect(screen.queryByText('Suggested life events')).not.toBeInTheDocument());
});

test('add creates the event, records the acceptance, and notifies the parent', async () => {
  vi.spyOn(lifeEventsApi, 'getLifeEventSuggestions').mockResolvedValue([suggestion]);
  const create = vi.spyOn(lifeEventsApi, 'createLifeEvent').mockResolvedValue({
    message: 'created',
    life_event: {
      ...suggestion,
      id: 'e1',
      created_at: '',
      updated_at: '',
      entity_id: 'contact-1',
      type: 'moved',
    },
  } as never);
  const resolve = vi
    .spyOn(lifeEventsApi, 'resolveLifeEventSuggestion')
    .mockResolvedValue(undefined);
  const onAccepted = vi.fn();
  render(<LifeEventSuggestions contactId={1} onAccepted={onAccepted} />);

  fireEvent.click(await screen.findByTestId('suggestion-accept'));

  await waitFor(() =>
    expect(create).toHaveBeenCalledWith({
      entity_id: 'contact-1',
      type: 'moved',
      category: 'home_living',
      date: { year: 2019 },
      end_date: { year: 2024 },
      source: 'ai-suggested',
    }),
  );
  await waitFor(() =>
    expect(resolve).toHaveBeenCalledWith({
      entity_id: 'contact-1',
      source_kind: 'address',
      source_entry_id: 'addr-1',
      event_type: 'moved',
      resolution: 'accepted',
    }),
  );
  await waitFor(() => expect(onAccepted).toHaveBeenCalled());
});
