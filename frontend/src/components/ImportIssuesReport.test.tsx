import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, expect, test } from 'vitest';
import '../i18n/config';
import ImportIssuesReport from './ImportIssuesReport';

afterEach(cleanup);

test('renders nothing when there are no issues', () => {
  const { container } = render(<ImportIssuesReport issues={[]} />);
  expect(container).toBeEmptyDOMElement();
});

test('labels skipped and failed rows and falls back to a dash for a missing name', () => {
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
  expect(screen.getByText('—')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Download as CSV' })).toBeInTheDocument();
});
