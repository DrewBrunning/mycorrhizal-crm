import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import type { ReactElement } from 'react';
import { useState } from 'react';
import { afterEach, describe, expect, test, vi } from 'vitest';
import '../i18n/config';
import { SnackbarProvider } from '../context/SnackbarContext';
import { DateFormatProvider } from '../DateFormatProvider';
import AddContactDialog from './AddContactDialog';
import AnniversaryDialog from './AnniversaryDialog';
import CadenceDialog from './CadenceDialog';
import ConversationAgendaDialog from './ConversationAgendaDialog';
import DataDecayDialog from './DataDecayDialog';
import type { TimelineItemValues } from './EditTimelineItemDialog';
import EditTimelineItemDialog from './EditTimelineItemDialog';
import FeedCreateDialog from './FeedCreateDialog';
import FieldDefinitionDialog from './FieldDefinitionDialog';
import GiftDialog from './GiftDialog';
import HouseholdDialog from './HouseholdDialog';
import LifeEventDialog from './LifeEventDialog';
import OccasionEventDialog from './OccasionEventDialog';
import OccasionObligationDialog from './OccasionObligationDialog';
import OrganizationDialog from './OrganizationDialog';
import PersonalInfoDialog from './PersonalInfoDialog';
import PreferenceDialog from './PreferenceDialog';
import RelationshipEdgeDialog from './RelationshipEdgeDialog';
import ReminderDialog from './ReminderDialog';
import ShareContactDialog from './ShareContactDialog';
import TitleDialog from './TitleDialog';
import WorkInformationDialog from './WorkInformationDialog';

// These dialogs fetch only when the test opens them in a particular mode
// (the note editor always loads contacts; sharing loads the user directory).
// Stub both at the module boundary so the discard behaviour is what is under
// test, not the data-loading path.
vi.mock('../api/contacts', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/contacts')>();
  return {
    ...actual,
    getContacts: vi.fn(async () => ({ contacts: [], next_cursor: '', limit: 40 })),
  };
});

vi.mock('../api/contactShares', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/contactShares')>();
  return {
    ...actual,
    getUserDirectory: vi.fn(async () => [{ id: 2, username: 'bob' }]),
  };
});

afterEach(cleanup);

function Providers({ children }: { children: ReactElement }) {
  return (
    <SnackbarProvider>
      <DateFormatProvider>{children}</DateFormatProvider>
    </SnackbarProvider>
  );
}

interface DialogCase {
  name: string;
  render: (onClose: () => void) => ReactElement;
  edit: () => void | Promise<void>;
  // Reads the value the user typed so "Keep editing" can prove it survived.
  readTypedValue: () => string;
  expectedTypedValue: string;
}

function textField(label: string, value: string): void {
  fireEvent.change(screen.getByLabelText(label), { target: { value } });
}

function readTextField(label: string): string {
  return (screen.getByLabelText(label) as HTMLInputElement).value;
}

function EditTimelineHarness({ onClose }: { onClose: () => void }) {
  const [values, setValues] = useState<TimelineItemValues>({ noteContent: 'Original note' });
  return (
    <EditTimelineItemDialog
      open
      onClose={onClose}
      onSave={vi.fn()}
      onDelete={vi.fn()}
      type="note"
      values={values}
      onChange={setValues}
      allContacts={[]}
    />
  );
}

const CASES: DialogCase[] = [
  {
    name: 'AddContactDialog',
    render: (onClose) => (
      <AddContactDialog
        open
        onClose={onClose}
        onContactAdded={vi.fn()}
        availableCircles={[]}
        availableTags={[]}
      />
    ),
    edit: () => textField('First Name *', 'Grace'),
    readTypedValue: () => readTextField('First Name *'),
    expectedTypedValue: 'Grace',
  },
  {
    name: 'ReminderDialog',
    render: (onClose) => (
      <ReminderDialog
        open
        onClose={onClose}
        onSave={vi.fn().mockResolvedValue(undefined)}
        contactId={1}
      />
    ),
    edit: () => textField('Message *', 'Call back'),
    readTypedValue: () => readTextField('Message *'),
    expectedTypedValue: 'Call back',
  },
  {
    name: 'GiftDialog',
    render: (onClose) => (
      <GiftDialog
        open
        onClose={onClose}
        onSave={vi.fn().mockResolvedValue(undefined)}
        gift={null}
        lifeEvents={[]}
        activities={[]}
      />
    ),
    edit: () => textField('What the gift is *', 'A scarf'),
    readTypedValue: () => readTextField('What the gift is *'),
    expectedTypedValue: 'A scarf',
  },
  {
    name: 'LifeEventDialog',
    render: (onClose) => (
      <LifeEventDialog
        open
        onClose={onClose}
        onSave={vi.fn().mockResolvedValue(undefined)}
        initial={{ type: 'A custom event' }}
      />
    ),
    edit: () => textField('Description', 'A life-changing year'),
    readTypedValue: () => readTextField('Description'),
    expectedTypedValue: 'A life-changing year',
  },
  {
    name: 'PreferenceDialog',
    render: (onClose) => (
      <PreferenceDialog open onClose={onClose} onSave={vi.fn().mockResolvedValue(undefined)} />
    ),
    edit: () => textField('Value *', 'Oat milk'),
    readTypedValue: () => readTextField('Value *'),
    expectedTypedValue: 'Oat milk',
  },
  {
    name: 'EditTimelineItemDialog',
    render: (onClose) => <EditTimelineHarness onClose={onClose} />,
    edit: () => textField('Content', 'Edited note'),
    readTypedValue: () => readTextField('Content'),
    expectedTypedValue: 'Edited note',
  },
  {
    name: 'OccasionEventDialog',
    render: (onClose) => (
      <OccasionEventDialog open onClose={onClose} onSave={vi.fn().mockResolvedValue(undefined)} />
    ),
    edit: () => textField('Title *', 'Annual dinner'),
    readTypedValue: () => readTextField('Title *'),
    expectedTypedValue: 'Annual dinner',
  },
  {
    name: 'OccasionObligationDialog',
    render: (onClose) => (
      <OccasionObligationDialog
        open
        onClose={onClose}
        onSave={vi.fn().mockResolvedValue(undefined)}
      />
    ),
    edit: () => textField('Label *', 'Send a card'),
    readTypedValue: () => readTextField('Label *'),
    expectedTypedValue: 'Send a card',
  },
  {
    name: 'HouseholdDialog',
    render: (onClose) => (
      <HouseholdDialog open onClose={onClose} onSave={vi.fn().mockResolvedValue(undefined)} />
    ),
    edit: () => textField('Name *', 'The Smiths'),
    readTypedValue: () => readTextField('Name *'),
    expectedTypedValue: 'The Smiths',
  },
  {
    name: 'RelationshipEdgeDialog',
    render: (onClose) => (
      <RelationshipEdgeDialog
        open
        onClose={onClose}
        onSave={vi.fn().mockResolvedValue(undefined)}
        viewedContactUid="viewed-uid"
      />
    ),
    edit: () => textField('Name *', 'Jane Doe'),
    readTypedValue: () => readTextField('Name *'),
    expectedTypedValue: 'Jane Doe',
  },
  {
    name: 'FieldDefinitionDialog',
    render: (onClose) => (
      <FieldDefinitionDialog open onClose={onClose} onSave={vi.fn().mockResolvedValue(undefined)} />
    ),
    edit: () => textField('Label *', 'Nickname source'),
    readTypedValue: () => readTextField('Label *'),
    expectedTypedValue: 'Nickname source',
  },
  {
    name: 'CadenceDialog',
    render: (onClose) => (
      <CadenceDialog
        open
        onClose={onClose}
        onSave={vi.fn().mockResolvedValue(undefined)}
        entityId="entity-uid"
      />
    ),
    edit: () => textField('Interval (days) *', '45'),
    readTypedValue: () => readTextField('Interval (days) *'),
    expectedTypedValue: '45',
  },
  {
    name: 'ConversationAgendaDialog',
    render: (onClose) => (
      <ConversationAgendaDialog
        open
        onClose={onClose}
        onSave={vi.fn().mockResolvedValue(undefined)}
      />
    ),
    edit: () => textField('What to bring up *', 'Ask about the new job'),
    readTypedValue: () => readTextField('What to bring up *'),
    expectedTypedValue: 'Ask about the new job',
  },
  {
    name: 'ShareContactDialog',
    render: (onClose) => <ShareContactDialog open onClose={onClose} vcardUID="contact-uid" />,
    edit: async () => {
      const combobox = screen.getByRole('combobox');
      fireEvent.mouseDown(combobox);
      fireEvent.click(await screen.findByRole('option', { name: 'bob' }));
    },
    readTypedValue: () => screen.getByRole('combobox').textContent ?? '',
    expectedTypedValue: 'bob',
  },
  {
    name: 'FeedCreateDialog',
    render: (onClose) => <FeedCreateDialog open onClose={onClose} />,
    edit: () => textField('Feed name *', 'My people'),
    readTypedValue: () => readTextField('Feed name *'),
    expectedTypedValue: 'My people',
  },
  {
    name: 'OrganizationDialog',
    render: (onClose) => (
      <OrganizationDialog open onClose={onClose} onSave={vi.fn().mockResolvedValue(undefined)} />
    ),
    edit: () => textField('Organization *', 'Analytical Engines Ltd'),
    readTypedValue: () => readTextField('Organization *'),
    expectedTypedValue: 'Analytical Engines Ltd',
  },
  {
    name: 'WorkInformationDialog',
    render: (onClose) => (
      <WorkInformationDialog open onClose={onClose} onSave={vi.fn().mockResolvedValue(undefined)} />
    ),
    edit: () => textField('Work Information', 'Works remotely'),
    readTypedValue: () => readTextField('Work Information'),
    expectedTypedValue: 'Works remotely',
  },
  {
    name: 'TitleDialog',
    render: (onClose) => (
      <TitleDialog open onClose={onClose} onSave={vi.fn().mockResolvedValue(undefined)} />
    ),
    edit: () => textField('Title *', 'Chief Engineer'),
    readTypedValue: () => readTextField('Title *'),
    expectedTypedValue: 'Chief Engineer',
  },
  {
    name: 'AnniversaryDialog',
    render: (onClose) => (
      <AnniversaryDialog open onClose={onClose} onSave={vi.fn().mockResolvedValue(undefined)} />
    ),
    edit: () => textField('Date *', '--06-15'),
    readTypedValue: () => readTextField('Date *'),
    expectedTypedValue: '--06-15',
  },
  {
    name: 'DataDecayDialog',
    render: (onClose) => (
      <DataDecayDialog
        open
        onClose={onClose}
        onSave={vi.fn().mockResolvedValue(undefined)}
        entityId="entity-uid"
      />
    ),
    edit: () => textField('Interval (days) *', '120'),
    readTypedValue: () => readTextField('Interval (days) *'),
    expectedTypedValue: '120',
  },
  {
    name: 'PersonalInfoDialog',
    render: (onClose) => (
      <PersonalInfoDialog open onClose={onClose} onSave={vi.fn().mockResolvedValue(undefined)} />
    ),
    edit: () => textField('Value *', 'Rock climbing'),
    readTypedValue: () => readTextField('Value *'),
    expectedTypedValue: 'Rock climbing',
  },
];

// Issue #1780: every create/edit dialog guards its close paths. One
// parameterised suite over all of them keeps a newly added dialog honest --
// add it here and it inherits the full Cancel/Keep editing/Discard/Escape
// contract.
for (const dialogCase of CASES) {
  describe(`${dialogCase.name} discard guard`, () => {
    test('an untouched dialog closes immediately, without a prompt', () => {
      const onClose = vi.fn();
      render(<Providers>{dialogCase.render(onClose)}</Providers>);

      fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));

      expect(onClose).toHaveBeenCalledTimes(1);
      expect(screen.queryByText('Discard unsaved changes?')).not.toBeInTheDocument();
    });

    test('Cancel after typing asks first; Keep editing preserves the draft; Discard closes', async () => {
      const onClose = vi.fn();
      render(<Providers>{dialogCase.render(onClose)}</Providers>);

      await dialogCase.edit();
      expect(dialogCase.readTypedValue()).toBe(dialogCase.expectedTypedValue);

      fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));

      expect(screen.getByText('Discard unsaved changes?')).toBeInTheDocument();
      expect(onClose).not.toHaveBeenCalled();

      fireEvent.click(screen.getByRole('button', { name: 'Keep editing' }));

      await waitFor(() =>
        expect(screen.queryByText('Discard unsaved changes?')).not.toBeInTheDocument(),
      );
      expect(onClose).not.toHaveBeenCalled();
      expect(dialogCase.readTypedValue()).toBe(dialogCase.expectedTypedValue);

      fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
      fireEvent.click(screen.getByRole('button', { name: 'Discard' }));

      expect(onClose).toHaveBeenCalledTimes(1);
    });

    test('Escape after typing asks first instead of discarding', async () => {
      const onClose = vi.fn();
      render(<Providers>{dialogCase.render(onClose)}</Providers>);

      await dialogCase.edit();

      fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape', code: 'Escape' });

      expect(screen.getByText('Discard unsaved changes?')).toBeInTheDocument();
      expect(onClose).not.toHaveBeenCalled();
    });
  });
}
