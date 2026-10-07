import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { useState } from 'react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { CardPersonalInfo } from '../api/contacts';
import PersonalInfoEditor from './PersonalInfoEditor';

// vitest here has no auto-cleanup and no globals — see CLAUDE.md.
afterEach(cleanup);

function renderEditor() {
  return render(
    <PersonalInfoEditor
      label="Personal Info"
      value={[{ kind: 'hobby', value: 'Gardening', level: 'high', label: '' }]}
      onChange={vi.fn()}
    />,
  );
}

// Both Selects used to be labelled with `t('contacts.personalInfo.kindOptions')`
// / `t('…levelOptions')` — the OBJECT nodes the individual options are nested
// under, not leaf strings. i18next has no string for an object node, so it
// rendered its own diagnostic as the visible field label:
//
//   key 'contacts.personalInfo.kindOptions (en)' returned an object instead of string.
//
// That shipped on the contact create and edit forms in all five languages.
test('labels the selects with translated strings, not raw i18n key paths', () => {
  renderEditor();

  expect(screen.getByLabelText('Type')).toBeInTheDocument();
  expect(screen.getByLabelText('Level')).toBeInTheDocument();

  // No i18next diagnostic text anywhere in the rendered output.
  expect(screen.queryByText(/returned an object instead of string/)).toBeNull();
  expect(screen.queryByText(/contacts\.personalInfo\./)).toBeNull();
});

test('renders the translated option label for the selected kind', () => {
  renderEditor();

  // The nested option leaves under kindOptions still resolve normally.
  expect(screen.getByText('Hobby')).toBeInTheDocument();
});

// Save-payload tests (issue #1482): every edit must report the exact Card
// payload the parent will persist, not merely render.
function Harness({
  initial,
  onChange,
}: {
  initial: CardPersonalInfo[];
  onChange: (next: CardPersonalInfo[]) => void;
}) {
  const [value, setValue] = useState(initial);
  return (
    <PersonalInfoEditor
      label="Personal Info"
      value={value}
      onChange={(n) => {
        setValue(n);
        onChange(n);
      }}
    />
  );
}
const last = (fn: ReturnType<typeof vi.fn>) => fn.mock.calls[fn.mock.calls.length - 1][0];

test('renders the group label and the Add button with no rows', () => {
  render(<PersonalInfoEditor label="Personal Info" value={[]} onChange={vi.fn()} />);
  expect(screen.getByText('Personal Info')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Add' })).toBeInTheDocument();
  expect(screen.queryByLabelText('Type')).toBeNull();
});

test('Add appends a blank hobby row', () => {
  const onChange = vi.fn();
  render(<Harness initial={[]} onChange={onChange} />);
  fireEvent.click(screen.getByRole('button', { name: 'Add' }));
  expect(last(onChange)).toEqual([{ kind: 'hobby', value: '' }]);
  expect(screen.getByLabelText('Value')).toHaveValue('');
});

test('editing value and label reports them while preserving the other fields', () => {
  const onChange = vi.fn();
  render(
    <Harness
      initial={[{ kind: 'expertise', value: 'Go', level: 'high', label: '' }]}
      onChange={onChange}
    />,
  );
  fireEvent.change(screen.getByLabelText('Value'), { target: { value: 'Rust' } });
  expect(last(onChange)).toEqual([{ kind: 'expertise', value: 'Rust', level: 'high', label: '' }]);
  fireEvent.change(screen.getByLabelText('Label'), { target: { value: 'systems' } });
  expect(last(onChange)).toEqual([
    { kind: 'expertise', value: 'Rust', level: 'high', label: 'systems' },
  ]);
});

test('changing the kind and level selects reports the chosen tokens', async () => {
  const onChange = vi.fn();
  render(<Harness initial={[{ kind: 'hobby', value: 'x' }]} onChange={onChange} />);

  fireEvent.mouseDown(screen.getAllByRole('combobox')[0]);
  fireEvent.click(await screen.findByRole('option', { name: 'Interest' }));
  expect(last(onChange)).toEqual([{ kind: 'interest', value: 'x' }]);

  fireEvent.mouseDown(screen.getAllByRole('combobox')[1]);
  fireEvent.click(await screen.findByRole('option', { name: 'Medium' }));
  expect(last(onChange)).toEqual([{ kind: 'interest', value: 'x', level: 'medium' }]);

  // The "No level" option clears it back to an empty string.
  fireEvent.mouseDown(screen.getAllByRole('combobox')[1]);
  fireEvent.click(await screen.findByRole('option', { name: 'No level' }));
  expect(last(onChange)).toEqual([{ kind: 'interest', value: 'x', level: '' }]);
});

test('Delete removes only the targeted row', () => {
  const onChange = vi.fn();
  const rows: CardPersonalInfo[] = [
    { kind: 'hobby', value: 'one' },
    { kind: 'interest', value: 'two' },
  ];
  render(<Harness initial={rows} onChange={onChange} />);
  fireEvent.click(screen.getAllByRole('button', { name: 'Delete' })[0]);
  expect(last(onChange)).toEqual([rows[1]]);
  expect(screen.queryByDisplayValue('one')).toBeNull();
  expect(screen.getByDisplayValue('two')).toBeInTheDocument();
});
