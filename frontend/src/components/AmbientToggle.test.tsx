import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import AmbientToggle from './AmbientToggle';

afterEach(cleanup);

test('renders the label and helper text, reflecting the checked state', () => {
  render(<AmbientToggle checked={true} onChange={vi.fn()} />);

  const toggle = screen.getByRole('switch', { name: 'Ambient contact — I see them all the time' });
  expect(toggle).toBeChecked();
  expect(toggle).toHaveAccessibleDescription(
    'Counts as in regular contact without logging activities',
  );
});

test('reports the next value when toggled on and off', () => {
  const onChange = vi.fn();
  const { rerender } = render(<AmbientToggle checked={false} onChange={onChange} />);

  fireEvent.click(screen.getByRole('switch'));
  expect(onChange).toHaveBeenLastCalledWith(true);

  rerender(<AmbientToggle checked={true} onChange={onChange} />);
  fireEvent.click(screen.getByRole('switch'));
  expect(onChange).toHaveBeenLastCalledWith(false);
});
