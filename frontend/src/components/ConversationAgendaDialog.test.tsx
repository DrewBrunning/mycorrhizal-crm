import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { ConversationAgenda } from '../api/conversationAgenda';
import ConversationAgendaDialog from './ConversationAgendaDialog';

afterEach(cleanup);

const item: ConversationAgenda = {
  id: 'agenda-1',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  entity_id: 'alice-uid',
  content: 'Ask about her mother\u2019s surgery',
  reference_url: 'https://example.com/article',
};

function renderDialog(props: Partial<React.ComponentProps<typeof ConversationAgendaDialog>> = {}) {
  const defaults: React.ComponentProps<typeof ConversationAgendaDialog> = {
    open: true,
    onClose: vi.fn(),
    onSave: vi.fn().mockResolvedValue(undefined),
    item,
    ...props,
  };
  return render(<ConversationAgendaDialog {...defaults} />);
}

test('prefills the content and reference url from the item', () => {
  renderDialog();
  expect(screen.getByLabelText('What to bring up *')).toHaveValue('Ask about her mother’s surgery');
  expect(screen.getByLabelText('Link (optional)')).toHaveValue('https://example.com/article');
});

test('save submits trimmed content and the reference url', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave });

  const contentInput = screen.getByLabelText('What to bring up *');
  fireEvent.change(contentInput, { target: { value: '  Ask about the garden  ' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await vi.waitFor(() => expect(onSave).toHaveBeenCalled());
  expect(onSave.mock.calls[0][0]).toEqual({
    content: 'Ask about the garden',
    reference_url: 'https://example.com/article',
  });
});

test('an empty content is rejected without calling onSave', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave, item: null });

  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await vi.waitFor(() =>
    expect(screen.getByText(/Enter what you want to bring up/i)).toBeInTheDocument(),
  );
  expect(onSave).not.toHaveBeenCalled();
});

test('a non-http scheme reference url is rejected client-side without calling onSave', async () => {
  // safeurl accepted mailto:; the agenda reference_url is http(s)-only now
  // (T41), and the dialog must catch it with a readable message rather than
  // a 400.
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ item: null, onSave });

  fireEvent.change(screen.getByLabelText('What to bring up *'), {
    target: { value: 'Ask about the article' },
  });
  fireEvent.change(screen.getByLabelText('Link (optional)'), {
    target: { value: 'mailto:a@b.com' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await vi.waitFor(() => expect(screen.getByText(/http:\/\/ or https:\/\//i)).toBeInTheDocument());
  expect(onSave).not.toHaveBeenCalled();
});

test('a scheme-less reference url is normalized to https so it renders as a link', async () => {
  // Mirror of GiftDialog's call: the backend's httpurl validator rejects a
  // value with no scheme, so the dialog defaults one to https.
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ item: null, onSave });

  fireEvent.change(screen.getByLabelText('What to bring up *'), {
    target: { value: 'Ask about the article' },
  });
  fireEvent.change(screen.getByLabelText('Link (optional)'), {
    target: { value: 'example.com/article' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await vi.waitFor(() => expect(onSave).toHaveBeenCalled());
  expect(onSave.mock.calls[0][0]).toEqual({
    content: 'Ask about the article',
    reference_url: 'https://example.com/article',
  });
});
