import { test, expect, afterEach, vi } from 'vitest';
import { render, screen, cleanup } from '@testing-library/react';
import '../i18n/config';
import PersonalInfoEditor from './PersonalInfoEditor';

// vitest here has no auto-cleanup and no globals — see CLAUDE.md.
afterEach(cleanup);

function renderEditor() {
  return render(
    <PersonalInfoEditor
      label="Personal Info"
      value={[{ kind: 'hobby', value: 'Gardening', level: 'high', label: '' }]}
      onChange={vi.fn()}
    />
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
