import { test, expect, vi, afterEach } from 'vitest';
import { render, screen, cleanup, fireEvent, waitFor } from '@testing-library/react';
import '../i18n/config';
import ExportFieldPickerDialog from './ExportFieldPickerDialog';
import { EXPORT_FIELD_SECTIONS, ExportFormat, ExportSelection } from '../api/export';

afterEach(cleanup);

function renderDialog(props: Partial<React.ComponentProps<typeof ExportFieldPickerDialog>> = {}) {
  const defaults: React.ComponentProps<typeof ExportFieldPickerDialog> = {
    open: true,
    onClose: vi.fn(),
    onExport: vi.fn().mockResolvedValue(undefined),
    ...props,
  };
  return render(<ExportFieldPickerDialog {...defaults} />);
}

function checkbox(name: string) {
  return screen.getByRole('checkbox', { name });
}

test('ordinary sections are checked by default; sensitive sections start locked', () => {
  renderDialog();

  // Ordinary, opt-out sections: on by default.
  expect(checkbox('Emails')).toBeChecked();
  expect(checkbox('Phones')).toBeChecked();
  expect(checkbox('Addresses')).toBeChecked();

  // Sensitivity-marked sections: locked AND disabled — not merely unchecked.
  const relationships = checkbox('Relationships');
  expect(relationships).toBeDisabled();
  expect(relationships).not.toBeChecked();
  expect(checkbox('Personal info')).toBeDisabled();
  expect(checkbox('Custom fields')).toBeDisabled();
});

test('a locked sensitive control stays untouched by clicks', () => {
  renderDialog();

  const relationships = checkbox('Relationships');
  fireEvent.click(relationships);
  expect(relationships).toBeDisabled();
  expect(relationships).not.toBeChecked();
});

// The binding foot-gun guard: the sensitive control is
// not interactive until the deliberate reveal action is taken.
test('sensitive controls become interactive only after the reveal action', () => {
  renderDialog();

  const relationships = checkbox('Relationships');
  expect(relationships).toBeDisabled();

  // Click the reveal button -> confirmation dialog.
  fireEvent.click(screen.getByRole('button', { name: 'Enable sensitive fields' }));
  expect(screen.getByText('Include sensitive data?')).toBeInTheDocument();

  // The reveal has not happened yet: still disabled.
  expect(relationships).toBeDisabled();

  // Confirm the deliberate action.
  fireEvent.click(screen.getByRole('button', { name: 'Enable' }));

  // Now the control is interactive.
  expect(relationships).toBeEnabled();
  fireEvent.click(relationships);
  expect(relationships).toBeChecked();
});

test('cancelling the reveal keeps the controls locked', async () => {
  renderDialog();

  fireEvent.click(screen.getByRole('button', { name: 'Enable sensitive fields' }));
  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));

  // The confirm dialog must finish closing (and lift its aria-hidden on the
  // main dialog) before the main dialog's controls are queryable again.
  await waitFor(() => expect(screen.queryByText('Include sensitive data?')).not.toBeInTheDocument());
  expect(await screen.findByRole('checkbox', { name: 'Relationships' })).toBeDisabled();
});

test('export sends only the selected sections and includeSensitive=false by default', async () => {
  const onExport = vi.fn().mockResolvedValue(undefined);
  const onClose = vi.fn();
  renderDialog({ onExport, onClose });

  // Deselect one ordinary section to prove the selection reflects the UI.
  fireEvent.click(checkbox('Notes'));

  fireEvent.click(screen.getByRole('button', { name: 'Export' }));

  const expectedSections = EXPORT_FIELD_SECTIONS.filter((s) => !s.sensitive).map((s) => s.token);
  await vi.waitFor(() => expect(onExport).toHaveBeenCalled());
  expect(onExport).toHaveBeenCalledWith('vcf4', {
    sections: expectedSections.filter((t) => t !== 'notes'),
    includeSensitive: false,
  });
  expect(onClose).toHaveBeenCalled();
});

test('exporting a revealed sensitive section sends the explicit opt-in', async () => {
  const onExport = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onExport });

  fireEvent.click(screen.getByRole('button', { name: 'Enable sensitive fields' }));
  fireEvent.click(screen.getByRole('button', { name: 'Enable' }));

  const relationships = await screen.findByRole('checkbox', { name: 'Relationships' });
  fireEvent.click(relationships);
  fireEvent.click(screen.getByRole('button', { name: 'Export' }));

  await vi.waitFor(() => expect(onExport).toHaveBeenCalled());
  const [format, selection] = onExport.mock.calls[0] as [ExportFormat, ExportSelection];
  expect(format).toBe('vcf4');
  expect(selection.sections).toContain('related_to');
  expect(selection.includeSensitive).toBe(true);
});

test('revealing but not selecting a sensitive section does NOT imply the opt-in', async () => {
  const onExport = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onExport });

  fireEvent.click(screen.getByRole('button', { name: 'Enable sensitive fields' }));
  fireEvent.click(screen.getByRole('button', { name: 'Enable' }));
  await waitFor(() => expect(screen.queryByText('Include sensitive data?')).not.toBeInTheDocument());
  // No sensitive section selected.

  fireEvent.click(screen.getByRole('button', { name: 'Export' }));

  await vi.waitFor(() => expect(onExport).toHaveBeenCalled());
  const [, selection] = onExport.mock.calls[0] as [ExportFormat, ExportSelection];
  expect(selection.includeSensitive).toBe(false);
});

test('the chosen format flows through to the export call', async () => {
  const onExport = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onExport });

  fireEvent.click(screen.getByLabelText('JSContact (.json)'));
  fireEvent.click(screen.getByRole('button', { name: 'Export' }));

  await vi.waitFor(() => expect(onExport).toHaveBeenCalled());
  expect(onExport.mock.calls[0][0]).toBe('jscontact');
});

test('export button is disabled when no sections are selected', () => {
  renderDialog();

  const exportButton = screen.getByRole('button', { name: 'Export' });
  expect(exportButton).toBeEnabled();

  for (const s of EXPORT_FIELD_SECTIONS.filter((x) => !x.sensitive)) {
    fireEvent.click(checkbox(labelFor(s.token)));
  }
  expect(screen.getByRole('button', { name: 'Export' })).toBeDisabled();
});

function labelFor(token: string): string {
  const labels: Record<string, string> = {
    emails: 'Emails',
    phones: 'Phones',
    addresses: 'Addresses',
    organizations: 'Organizations',
    anniversaries: 'Anniversaries',
    media: 'Photos',
    online_services: 'Online services',
    links: 'Links',
    notes: 'Notes',
    keywords: 'Tags',
    speak_to_as: 'Pronouns & addressing',
    members: 'Group members',
    languages: 'Preferred languages',
  };
  return labels[token];
}
