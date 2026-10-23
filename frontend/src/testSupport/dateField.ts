import { fireEvent, screen, waitFor } from '@testing-library/react';
import { expect } from 'vitest';

/**
 * Open a DateField by its label and choose `iso` from the month calendar.
 * `fromIso` is the field's current value (the picker opens on that month);
 * omit it for an empty field, whose picker opens on "today's" month.
 */
export async function pickDate(label: string, iso: string, fromIso?: string): Promise<void> {
  fireEvent.click(screen.getByRole('textbox', { name: label }));
  const dialogName = label.replace(/ \*$/, '');
  const dialog = screen.getByRole('dialog', { name: dialogName });
  const target = new Date(`${iso}T00:00:00`);
  const base = fromIso ? new Date(`${fromIso}T00:00:00`) : new Date();
  const months =
    (target.getFullYear() - base.getFullYear()) * 12 + (target.getMonth() - base.getMonth());
  const arrow = months < 0 ? 'Previous month' : 'Next month';
  for (let i = 0; i < Math.abs(months); i++) {
    fireEvent.click(screen.getByRole('button', { name: arrow }));
  }
  const day = dialog.querySelector(`[data-date="${iso}"]`);
  if (!day) throw new Error(`No day cell for ${iso} in the ${label} picker`);
  fireEvent.click(day);
  await waitFor(() => expect(screen.queryByRole('dialog', { name: dialogName })).toBeNull());
}
