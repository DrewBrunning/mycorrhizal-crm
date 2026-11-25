import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { SnackbarProvider } from '../context/SnackbarContext';
import ReviewDuplicatesDialog from './ReviewDuplicatesDialog';

// The merge dialog has its own tests (MergeContactsDialog.test.tsx). Here it is
// a stub whose two buttons drive this dialog's onClose/onMerged handling, which
// ReviewDuplicatesDialog.test.tsx (real merge dialog) does not reach.
vi.mock('./MergeContactsDialog', () => ({
  default: (props: { onClose: () => void; onMerged: (id: number) => void }) => (
    <div role="dialog" aria-label="merge-stub">
      <button type="button" onClick={props.onClose}>
        merge-close
      </button>
      <button type="button" onClick={() => props.onMerged(1)}>
        merge-done
      </button>
    </div>
  ),
}));

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

const summary = (
  id: number,
  uid: string,
  firstname: string,
  lastname: string,
  over: Record<string, unknown> = {},
) => ({
  id,
  uid,
  firstname,
  lastname,
  nickname: '',
  fn: `${firstname} ${lastname}`,
  primary_email: '',
  primary_phone: '',
  birthday: '',
  org: '',
  photo: '',
  photo_thumbnail: '',
  archived: false,
  ...over,
});

const pairsResponse = (pairs: unknown[]) => ({ pairs, total: pairs.length, page: 1, limit: 100 });

const json = (status: number, body: unknown) => ({
  ok: status < 400,
  status,
  headers: new Headers({ 'content-type': 'application/json' }),
  json: async () => body,
  text: async () => JSON.stringify(body),
});

function mockFetch(pairs: () => unknown[], dismissStatus = 200) {
  const calls = { scans: 0, dismisses: 0 };
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      if (url.includes('/contacts/duplicates/dismiss')) {
        calls.dismisses += 1;
        return dismissStatus < 400
          ? json(200, { message: 'Pair dismissed' })
          : json(dismissStatus, { error: 'boom', code: 'INTERNAL_ERROR' });
      }
      if (url.includes('/contacts/duplicates')) {
        calls.scans += 1;
        return json(200, pairsResponse(pairs()));
      }
      throw new Error(`unexpected fetch: ${url}`);
    }),
  );
  return calls;
}

function renderDialog() {
  return render(
    <SnackbarProvider>
      <ReviewDuplicatesDialog open onClose={vi.fn()} />
    </SnackbarProvider>,
  );
}

const pair = (over: Record<string, unknown> = {}) => ({
  a: summary(1, 'alice-uid', 'Alice', 'Adams'),
  b: summary(2, 'bob-uid', 'Bob', 'Brown'),
  reasons: ['name'],
  confidence: 0.5,
  ...over,
});

test('labels phone and unrecognised reasons, and falls back for nameless or archived contacts', async () => {
  mockFetch(() => [
    pair({
      a: summary(1, 'nameless-uid', '', ''),
      b: summary(2, 'bob-uid', 'Bob', 'Brown', { archived: true }),
      // An unknown reason token from a newer server is shown as-is.
      reasons: ['phone', 'birthday'],
      confidence: 0.95,
    }),
  ]);
  renderDialog();

  expect(await screen.findByText('Same phone')).toBeInTheDocument();
  expect(screen.getByText('birthday')).toBeInTheDocument();
  // No name: the uid stands in, so the row is never blank.
  expect(screen.getByText('nameless-uid')).toBeInTheDocument();
  // Archived with no email/phone: the caption and a chip both say so.
  expect(screen.getAllByText('Archived')).toHaveLength(2);
  expect(screen.getByText('95% confidence')).toBeInTheDocument();
});

test('cancelling the confirmation dismisses nothing', async () => {
  const calls = mockFetch(() => [pair()]);
  renderDialog();
  await screen.findByText('Alice Adams');
  vi.spyOn(window, 'confirm').mockReturnValue(false);

  fireEvent.click(screen.getByRole('button', { name: 'Not a duplicate' }));

  expect(calls.dismisses).toBe(0);
  expect(screen.getByText('Alice Adams')).toBeInTheDocument();
});

test('a failed dismiss keeps the pair and drops its focus request', async () => {
  let pairs = [pair()];
  const calls = mockFetch(() => pairs, 500);
  renderDialog();
  await screen.findByText('Alice Adams');
  vi.spyOn(window, 'confirm').mockReturnValue(true);
  const button = screen.getByRole('button', { name: 'Not a duplicate' });
  button.focus();

  fireEvent.click(button);

  await waitFor(() => expect(calls.dismisses).toBe(1));
  await waitFor(() => expect(button).not.toBeDisabled());
  expect(screen.getByText('Alice Adams')).toBeInTheDocument();
  expect(document.activeElement).toBe(button);

  // The failed dismiss must not leave a pending focus move behind: a later,
  // unrelated list change (a re-scan after a merge) must not pull focus to
  // the dialog title.
  const merge = screen.getByRole('button', { name: 'Merge' });
  fireEvent.click(merge);
  pairs = [];
  const stub = await screen.findByRole('dialog', { name: 'merge-stub' });
  const done = within(stub).getByText('merge-done');
  done.focus();
  fireEvent.click(done);
  await screen.findByText('No duplicate candidates found.');
  expect(document.activeElement).not.toBe(screen.getByText('Review duplicates'));
});

test('closing the merge dialog returns to the list without a re-scan', async () => {
  const calls = mockFetch(() => [pair()]);
  renderDialog();
  await screen.findByText('Alice Adams');
  const scansBefore = calls.scans;

  fireEvent.click(screen.getByRole('button', { name: 'Merge' }));
  fireEvent.click(
    within(await screen.findByRole('dialog', { name: 'merge-stub' })).getByText('merge-close'),
  );

  await waitFor(() =>
    expect(screen.queryByRole('dialog', { name: 'merge-stub' })).not.toBeInTheDocument(),
  );
  expect(calls.scans).toBe(scansBefore);
});

test('a completed merge closes the merge dialog and re-runs the scan', async () => {
  let pairs = [pair()];
  const calls = mockFetch(() => pairs);
  renderDialog();
  await screen.findByText('Alice Adams');
  const scansBefore = calls.scans;

  fireEvent.click(screen.getByRole('button', { name: 'Merge' }));
  pairs = [];
  fireEvent.click(
    within(await screen.findByRole('dialog', { name: 'merge-stub' })).getByText('merge-done'),
  );

  await screen.findByText('No duplicate candidates found.');
  expect(calls.scans).toBe(scansBefore + 1);
  expect(screen.queryByRole('dialog', { name: 'merge-stub' })).not.toBeInTheDocument();
});
