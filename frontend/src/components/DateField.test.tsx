import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { useState } from 'react';
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
  fireEvent.click(screen.getByRole('button', { name: 'Open calendar' }));
  return screen.getByRole('dialog', { name: 'Start date' });
}

describe('DateField display', () => {
  test('shows the format hint as the placeholder when empty', () => {
    renderField();
    expect(input()).toHaveValue('');
    expect(input()).toHaveAttribute('placeholder', 'DD.MM.YYYY');
  });

  test('advertises the dialog without an unsupported aria-expanded on the textbox', () => {
    renderField();
    expect(input()).toHaveAttribute('aria-haspopup', 'dialog');
    expect(input()).not.toHaveAttribute('aria-expanded');
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
    fireEvent.keyDown(input(), { key: 'ArrowDown' });
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

test('a required field exposes its label and required state to assistive tech', () => {
  renderField({ required: true });
  const field = screen.getByRole('textbox', { name: 'Start date *' });
  expect(field).toHaveAttribute('aria-required', 'true');
});

function type(text: string) {
  fireEvent.focus(input());
  fireEvent.change(input(), { target: { value: text } });
}

describe('DateField keyboard entry (issue #2198)', () => {
  test('typing a full date in the Date format preference reports YYYY-MM-DD', () => {
    const { onChange } = renderField();
    type('15.01.2026');
    expect(onChange).toHaveBeenLastCalledWith('2026-01-15');
  });

  test('a far-past date can be typed without calendar navigation', () => {
    const { onChange } = renderField();
    type('03.07.1950');
    expect(onChange).toHaveBeenLastCalledWith('1950-07-03');
  });

  test.each([
    ['us', '01/15/2026'],
    ['us-mmmm', '01/15/2026'],
    ['iso', '2026-01-15'],
    ['ca', '15/01/2026'],
    ['eu-hyphen', '15-01-2026'],
    ['eu-mmm', '15.01.2026'],
  ] as const)('parses the %s format', (format, text) => {
    setDateFormatPreference(format);
    const { onChange } = renderField();
    type(text);
    expect(onChange).toHaveBeenLastCalledWith('2026-01-15');
  });

  test('shows the numeric form while editing a month-name format', () => {
    setDateFormatPreference('us-mmmm');
    renderField({ value: '2026-01-15' });
    expect(input()).toHaveValue('January 15, 2026');
    fireEvent.focus(input());
    expect(input()).toHaveValue('01/15/2026');
  });

  test('normalizes the typed text on blur', () => {
    renderField({ value: '2026-01-15' });
    type('1.2.2026');
    fireEvent.blur(input());
    expect(input()).toHaveValue('15.01.2026');
  });

  test('an impossible date shows an error and leaves the value unchanged', () => {
    const { onChange } = renderField({ value: '2026-01-15' });
    type('31.02.2026');
    fireEvent.blur(input());
    expect(onChange).not.toHaveBeenCalled();
    expect(input()).toHaveAttribute('aria-invalid', 'true');
    expect(screen.getByText('Enter a valid date as DD.MM.YYYY.')).toBeInTheDocument();
  });

  test('text that is not a date is rejected, and so is a year-less date', () => {
    const { onChange } = renderField({ value: '2026-01-15' });
    type('hello');
    fireEvent.blur(input());
    expect(screen.getByText('Enter a valid date as DD.MM.YYYY.')).toBeInTheDocument();
    type('15.01.');
    fireEvent.blur(input());
    expect(screen.getByText('Enter a valid date as DD.MM.YYYY.')).toBeInTheDocument();
    expect(onChange).not.toHaveBeenCalled();
  });

  test('a date outside min/max is rejected', () => {
    const { onChange } = renderField({ min: '2026-01-10', max: '2026-01-20' });
    type('25.01.2026');
    fireEvent.blur(input());
    expect(onChange).not.toHaveBeenCalled();
    expect(screen.getByText('That date is outside the allowed range.')).toBeInTheDocument();
  });

  test('the error clears once the text is corrected', () => {
    const { onChange } = renderField();
    type('99.99.2026');
    fireEvent.blur(input());
    expect(input()).toHaveAttribute('aria-invalid', 'true');
    type('15.01.2026');
    expect(onChange).toHaveBeenLastCalledWith('2026-01-15');
    expect(input()).not.toHaveAttribute('aria-invalid', 'true');
  });

  test('Enter commits typed text without opening the picker', () => {
    const { onChange } = renderField();
    type('15.01.2026');
    fireEvent.keyDown(input(), { key: 'Enter' });
    expect(onChange).toHaveBeenLastCalledWith('2026-01-15');
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  test('emptying the text clears the value', () => {
    const { onChange } = renderField({ value: '2026-01-15' });
    type('');
    expect(onChange).toHaveBeenLastCalledWith('');
  });

  test('a picked day replaces any half-typed text', async () => {
    function Controlled() {
      const [value, setValue] = useState('');
      return <DateField label="Start date" value={value} onChange={setValue} />;
    }
    render(
      <DateFormatProvider>
        <Controlled />
      </DateFormatProvider>,
    );
    type('15.0');
    await pickDate('Start date', '2026-01-01');
    expect(input()).toHaveValue('01.01.2026');
  });
});

describe('DateField fixed-height calendar (issue #2199)', () => {
  test.each([
    ['a 5-week month', '2026-01-15'],
    ['a 4-week month', '2027-02-10'],
    ['a 6-week month', '2026-08-10'],
  ])('renders 6 week rows for %s', (_name, value) => {
    renderField({ value });
    const dialog = openPicker();
    expect(dialog.querySelectorAll('[data-cell]')).toHaveLength(42);
  });
});
