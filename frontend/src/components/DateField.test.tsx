import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import '../i18n/config';
import { DateFormatProvider, setDateFormatPreference } from '../DateFormatProvider';
import { pickDate } from '../testSupport/dateField';
import DateField from './DateField';

afterEach(cleanup);

beforeEach(() => {
  setDateFormatPreference('eu');
});

function renderField(props: Partial<React.ComponentProps<typeof DateField>> = {}) {
  const onChange = vi.fn();
  render(
    <DateFormatProvider>
      <DateField label="Start date" value="" onChange={onChange} {...props} />
    </DateFormatProvider>,
  );
  return { onChange };
}

function input() {
  return screen.getByRole('textbox', { name: 'Start date' });
}

function openPicker() {
  fireEvent.click(input());
  return screen.getByRole('dialog', { name: 'Start date' });
}

describe('DateField display', () => {
  test('shows the format hint as the placeholder when empty', () => {
    renderField();
    expect(input()).toHaveValue('');
    expect(input()).toHaveAttribute('placeholder', 'DD.MM.YYYY');
  });

  test('renders the selected day in the Date format preference, not mm/dd/yyyy', () => {
    renderField({ value: '2026-01-15' });
    expect(input()).toHaveValue('15.01.2026');

    cleanup();
    setDateFormatPreference('us');
    renderField({ value: '2026-01-15' });
    expect(input()).toHaveValue('01/15/2026');

    cleanup();
    setDateFormatPreference('us-mmmm');
    renderField({ value: '2026-01-15' });
    expect(input()).toHaveValue('January 15, 2026');
  });
});

describe('DateField picker', () => {
  test('selects a day, reports it as YYYY-MM-DD, and closes', async () => {
    const { onChange } = renderField();

    await pickDate('Start date', '2026-01-01');

    expect(onChange).toHaveBeenCalledWith('2026-01-01');
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  test('clears a selected day', async () => {
    const { onChange } = renderField({ value: '2026-01-15' });
    openPicker();

    fireEvent.click(screen.getByRole('button', { name: 'Clear' }));

    expect(onChange).toHaveBeenCalledWith('');
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
  });

  test('navigates by month', () => {
    renderField({ value: '2026-01-15' });
    openPicker();

    expect(screen.getByText('January 2026')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Next month' }));
    expect(screen.getByText('February 2026')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Previous month' }));
    fireEvent.click(screen.getByRole('button', { name: 'Previous month' }));
    expect(screen.getByText('December 2025')).toBeInTheDocument();
  });

  test('disables days outside min/max', () => {
    renderField({ value: '2026-01-15', min: '2026-01-10', max: '2026-01-20' });
    openPicker();

    expect(screen.getByRole('button', { name: '09.01.2026' })).toBeDisabled();
    expect(screen.getByRole('button', { name: '10.01.2026' })).not.toBeDisabled();
    expect(screen.getByRole('button', { name: '20.01.2026' })).not.toBeDisabled();
    expect(screen.getByRole('button', { name: '21.01.2026' })).toBeDisabled();
  });

  test('opens from the keyboard', () => {
    renderField();
    fireEvent.keyDown(input(), { key: 'Enter' });
    expect(screen.getByRole('dialog', { name: 'Start date' })).toBeInTheDocument();
  });

  test('marks the selected day and today', () => {
    renderField({ value: '2026-01-15' });
    openPicker();

    expect(screen.getByRole('button', { name: '15.01.2026' })).toHaveAttribute(
      'aria-pressed',
      'true',
    );
    expect(screen.getByRole('button', { name: '14.01.2026' })).toHaveAttribute(
      'aria-pressed',
      'false',
    );
  });
});

test('closes on Escape', async () => {
  renderField();
  openPicker();
  fireEvent.keyDown(screen.getByRole('dialog', { name: 'Start date' }), { key: 'Escape' });
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
});
