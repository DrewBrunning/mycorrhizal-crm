import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import LanguageField from './LanguageField';

afterEach(cleanup);

// Issue #1725: the single card-language field accepts a BCP-47 tag, so it has
// to show what one looks like rather than relying on the info tooltip alone.
test('shows a BCP-47 tag example in the placeholder and helper text', () => {
  render(<LanguageField value="" onChange={vi.fn()} />);

  const input = screen.getByRole('combobox', { name: 'Language' });
  expect(input).toHaveAttribute('placeholder', 'de-AT');
  expect(screen.getByText('BCP-47 tag, e.g. de, en-US, de-AT.')).toBeInTheDocument();
});
