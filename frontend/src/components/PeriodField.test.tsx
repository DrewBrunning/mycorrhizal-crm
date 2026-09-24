import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import PeriodField from './PeriodField';

// No globals/auto-cleanup in this vitest setup (CLAUDE.md frontend trap #1).
afterEach(cleanup);

test('renders an existing range and saves edited years', async () => {
  const onSave = vi.fn(async () => {});
  render(
    <PeriodField
      label="Period"
      range={{ start: { year: 2019 }, end: { year: 2024 } }}
      onSave={onSave}
    />,
  );

  expect(screen.getByText('2019 – 2024')).toBeInTheDocument();

  fireEvent.click(screen.getByTestId('period-edit'));
  fireEvent.change(screen.getByLabelText('From (year)'), { target: { value: '2018' } });
  fireEvent.change(screen.getByLabelText('To (year)'), { target: { value: '2025' } });
  fireEvent.click(screen.getByTestId('period-save'));

  await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
  expect(onSave).toHaveBeenCalledWith({ start: { year: 2018 }, end: { year: 2025 } });
});

test('renders open-ended ranges', () => {
  const { rerender } = render(
    <PeriodField label="Period" range={{ start: { year: 2019 } }} onSave={vi.fn()} />,
  );
  expect(screen.getByText('Since 2019')).toBeInTheDocument();

  rerender(<PeriodField label="Period" range={{ end: { year: 2024 } }} onSave={vi.fn()} />);
  expect(screen.getByText('Until 2024')).toBeInTheDocument();
});

test('saving an empty range clears the period', async () => {
  const onSave = vi.fn(async () => {});
  render(
    <PeriodField
      label="Period"
      range={{ start: { year: 2019 }, end: { year: 2024 } }}
      onSave={onSave}
    />,
  );

  fireEvent.click(screen.getByTestId('period-edit'));
  fireEvent.change(screen.getByLabelText('From (year)'), { target: { value: '' } });
  fireEvent.change(screen.getByLabelText('To (year)'), { target: { value: '' } });
  fireEvent.click(screen.getByTestId('period-save'));

  await waitFor(() => expect(onSave).toHaveBeenCalledWith(undefined));
});
