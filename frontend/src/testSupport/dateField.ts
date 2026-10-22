import { fireEvent, screen, waitFor } from '@testing-library/react';
import { expect } from 'vitest';

/**
 * Open a DateField by its label and choose `iso` from the month calendar.
 * Assumes the picker opens on "today's" month (the empty-value case).
 */
export async function pickDate(label: string, iso: string): Promise<void> {
  fireEvent.click(screen.getByRole('textbox', { name: label }));
  const dialog = screen.getByRole('dialog', { name: label });
  const target = new Date(`${iso}T00:00:00`);
  const now = new Date();
  const months =
    (target.getFullYear() - now.getFullYear()) * 12 + (target.getMonth() - now.getMonth());
  const arrow = months < 0 ? 'Previous month' : 'Next month';
  for (let i = 0; i < Math.abs(months); i++) {
    fireEvent.click(screen.getByRole('button', { name: arrow }));
  }
  const day = dialog.querySelector(`[data-date="${iso}"]`);
  if (!day) throw new Error(`No day cell for ${iso} in the ${label} picker`);
  fireEvent.click(day);
  await waitFor(() => expect(screen.queryByRole('dialog', { name: label })).toBeNull());
}
