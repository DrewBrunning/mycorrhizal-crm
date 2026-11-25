import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import i18n from '../i18n/config';
import ImportIssuesReport from './ImportIssuesReport';

afterEach(cleanup);

test('renders nothing when there are no issues', () => {
  const { container } = render(<ImportIssuesReport issues={[]} />);
  expect(container).toBeEmptyDOMElement();
});

// #2950: import history never stores an email or phone (#2563), so a nameless row's stored name
// is blank; the table says so in words rather than showing a bare dash.
test('labels skipped and failed rows and says the name was not kept when it is missing', () => {
  render(
    <ImportIssuesReport
      issues={[
        { row: 2, name: 'Ann', email: '', phone: '', status: 'skipped', reason: 'duplicate' },
        { row: 5, name: '', email: '', phone: '', status: 'failed', reason: 'bad email' },
      ]}
    />,
  );
  expect(screen.getByText('Skipped')).toBeInTheDocument();
  expect(screen.getByText('Failed')).toBeInTheDocument();
  expect(screen.getByText('Name not kept')).toBeInTheDocument();
  expect(screen.queryByText('—')).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Download as CSV' })).toBeInTheDocument();
});

test('the not-kept name label follows the language', async () => {
  await i18n.changeLanguage('de');
  try {
    render(
      <ImportIssuesReport
        issues={[{ row: 5, name: '', email: '', phone: '', status: 'failed', reason: 'bad email' }]}
      />,
    );
    expect(screen.getByText('Name nicht gespeichert')).toBeInTheDocument();
  } finally {
    await i18n.changeLanguage('en');
  }
});

// --- #2741: the 'warning' status ----------------------------------------------

const warningIssue = {
  row: 3,
  name: 'Ada Lovelace',
  email: '',
  phone: '',
  status: 'warning' as const,
  concept: 'birthday',
  reason: 'birthday not imported: invalid value',
};

test('a warning row says Warning and explains the concept, not Skipped', () => {
  render(<ImportIssuesReport issues={[warningIssue]} />);
  expect(screen.getByText('Warning')).toBeInTheDocument();
  expect(screen.queryByText('Skipped')).not.toBeInTheDocument();
  expect(screen.getByText('Birthday not imported: invalid value')).toBeInTheDocument();
  // Only warnings: the rows were imported, so the table is not "not imported".
  expect(screen.getByRole('table', { name: 'Imported with warnings' })).toBeInTheDocument();
});

test('the warning explanation follows the concept and the language, not the English reason', async () => {
  await i18n.changeLanguage('de');
  try {
    render(
      <ImportIssuesReport
        issues={[warningIssue, { ...warningIssue, row: 4, concept: 'anniversary' }]}
      />,
    );
    expect(screen.getAllByText('Warnung')).toHaveLength(2);
    expect(screen.getByText('Geburtstag nicht importiert: ungültiger Wert')).toBeInTheDocument();
    expect(screen.getByText('Jahrestag nicht importiert: ungültiger Wert')).toBeInTheDocument();
    expect(screen.queryByText('birthday not imported: invalid value')).not.toBeInTheDocument();
  } finally {
    await i18n.changeLanguage('en');
  }
});

test('a warning with an unknown concept shows the server reason', () => {
  render(
    <ImportIssuesReport
      issues={[{ ...warningIssue, concept: 'nickname', reason: 'nickname was shortened' }]}
    />,
  );
  expect(screen.getByText('nickname was shortened')).toBeInTheDocument();
});

test('mixed reports count each kind and filter to one', () => {
  render(
    <ImportIssuesReport
      issues={[
        { row: 2, name: 'Ann', email: '', phone: '', status: 'skipped', reason: 'duplicate' },
        { row: 5, name: 'Bob', email: '', phone: '', status: 'failed', reason: 'bad email' },
        warningIssue,
      ]}
    />,
  );
  expect(screen.getByRole('table', { name: 'Rows to review' })).toBeInTheDocument();
  const notImported = screen.getByRole('button', { name: 'Not imported (2)' });
  const warnings = screen.getByRole('button', { name: 'Warning (1)' });
  expect(screen.getByRole('button', { name: 'All (3)' })).toHaveAttribute('aria-pressed', 'true');

  fireEvent.click(warnings);
  expect(screen.getByText('Ada Lovelace')).toBeInTheDocument();
  expect(screen.queryByText('Ann')).not.toBeInTheDocument();
  fireEvent.click(notImported);
  expect(screen.getByText('Ann')).toBeInTheDocument();
  expect(screen.getByText('Bob')).toBeInTheDocument();
  expect(screen.queryByText('Ada Lovelace')).not.toBeInTheDocument();
});

test('a report without warnings has no filter and keeps its title', () => {
  render(
    <ImportIssuesReport
      issues={[{ row: 2, name: 'Ann', email: '', phone: '', status: 'skipped', reason: 'dup' }]}
    />,
  );
  expect(screen.getByRole('table', { name: 'Rows not imported' })).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: /^All/ })).not.toBeInTheDocument();
});

test('the CSV download carries the worded warning status and concept', async () => {
  const createObjectURL = vi.fn((_blob: Blob) => 'blob:report');
  Object.assign(URL, { createObjectURL, revokeObjectURL: vi.fn() });
  render(<ImportIssuesReport issues={[warningIssue]} />);
  fireEvent.click(screen.getByRole('button', { name: 'Download as CSV' }));
  const blob = createObjectURL.mock.calls[0][0];
  const text = await new Promise<string>((resolve) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result));
    reader.readAsText(blob);
  });
  expect(text).toContain('Status,Reason,Concept');
  expect(text).toContain(',Warning,birthday not imported: invalid value,birthday');
});

test('pressing the selected filter again keeps it selected', () => {
  render(
    <ImportIssuesReport
      issues={[
        { row: 2, name: 'Ann', email: '', phone: '', status: 'skipped', reason: 'duplicate' },
        warningIssue,
      ]}
    />,
  );
  const all = screen.getByRole('button', { name: 'All (2)' });
  fireEvent.click(all);
  expect(all).toHaveAttribute('aria-pressed', 'true');
  expect(screen.getByText('Ann')).toBeInTheDocument();
});
