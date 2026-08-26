import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { CardOnlineService } from '../api/contacts';
import type { LinkFieldType } from '../api/linkFieldTypes';
import { resolveLinkFieldTypeIcon } from '../linkFieldTypeIcons';
import OnlineServiceEditor from './OnlineServiceEditor';

afterEach(cleanup);

function makeLinkFieldType(name: string, icon: string, id = name.toLowerCase()): LinkFieldType {
  return {
    id,
    name,
    protocol: `https://example.com/u/{value}`,
    category: 'social',
    icon,
    is_default: false,
    position: 0,
    created_at: '',
    updated_at: '',
  };
}

const WHATSAPP = makeLinkFieldType('WhatsApp', 'mdiWhatsapp');
const INSTAGRAM = makeLinkFieldType('Instagram', 'mdiInstagram');

const onChange = vi.fn();

function renderEditor(props: Partial<React.ComponentProps<typeof OnlineServiceEditor>> = {}) {
  onChange.mockReset();
  const defaults: React.ComponentProps<typeof OnlineServiceEditor> = {
    label: 'Social Profiles',
    value: [],
    onChange,
  };
  return render(<OnlineServiceEditor {...defaults} {...props} />);
}

// T44 — the registry must reach the editor: the service field is a freeSolo
// Autocomplete sourced from the user's LinkFieldType registry, with each
// option rendering its icon.
test("offers the user's LinkFieldType registry entries as service options with their icons", async () => {
  renderEditor({
    linkFieldTypes: [WHATSAPP, INSTAGRAM],
    value: [{ uri: 'https://example.com/u/alice' }],
  });

  fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Service' }));

  expect(await screen.findByRole('option', { name: 'WhatsApp' })).toBeInTheDocument();
  expect(screen.getByRole('option', { name: 'Instagram' })).toBeInTheDocument();

  // The dropdown option carries its registry icon, not just the name text.
  const whatsappOption = screen.getByRole('option', { name: 'WhatsApp' });
  const iconPath = whatsappOption.querySelector('svg path');
  expect(iconPath).not.toBeNull();
  expect(iconPath?.getAttribute('d')).toBe(resolveLinkFieldTypeIcon('mdiWhatsapp'));
});

// Selecting a suggested entry stores its canonical registry name.
test('selecting a registry option stores the canonical service name', async () => {
  renderEditor({
    value: [{ uri: 'https://wa.me/15551234567' }],
    linkFieldTypes: [WHATSAPP],
  });

  fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Service' }));
  fireEvent.click(await screen.findByRole('option', { name: 'WhatsApp' }));

  expect(onChange).toHaveBeenCalledWith([
    {
      id: undefined,
      uri: 'https://wa.me/15551234567',
      service: 'WhatsApp',
      user: undefined,
      label: undefined,
      contexts: undefined,
      pref: undefined,
    },
  ]);
});

// T44 trap — freeSolo must stay free-solo: an unregistered/one-off service
// name (e.g. "Company Slack") must still be typable and saved exactly like a
// plain text field, never locked to the registry.
test('typing an unregistered service name still saves (freeSolo preserved)', async () => {
  renderEditor({
    value: [{ uri: 'https://example.com/u/alice' }],
    linkFieldTypes: [WHATSAPP],
  });

  fireEvent.change(screen.getByRole('combobox', { name: 'Service' }), {
    target: { value: 'Company Slack' },
  });

  expect(onChange).toHaveBeenCalledWith([
    {
      id: undefined,
      uri: 'https://example.com/u/alice',
      service: 'Company Slack',
      user: undefined,
      label: undefined,
      contexts: undefined,
      pref: undefined,
    },
  ]);
});

// T44 — uriOnly is the IMPP shape: service + address, no separate user/handle.
test('uriOnly renders service and address without the user field', () => {
  renderEditor({ uriOnly: true, value: [{ uri: 'xmpp:alice@example.com' }] });

  expect(screen.getByRole('combobox', { name: 'Service' })).toBeInTheDocument();
  expect(screen.getByLabelText('Address')).toBeInTheDocument();
  expect(screen.queryByLabelText('Username')).toBeNull();
});

// Round-trip safety: editing one field of an existing entry preserves every
// other field (id, user, label, contexts, pref) through the adapter pair.
test('editing preserves the rest of the row (id/user/label/contexts/pref)', () => {
  const value: CardOnlineService[] = [
    {
      id: 'row-1',
      uri: 'xmpp:bob@example.com',
      service: 'WhatsApp',
      user: 'bob',
      label: 'Personal',
      contexts: ['work'],
      pref: 1,
    },
  ];
  renderEditor({ uriOnly: true, value });

  fireEvent.change(screen.getByLabelText('Address'), {
    target: { value: 'xmpp:robert@example.com' },
  });

  expect(onChange).toHaveBeenCalledWith([
    {
      id: 'row-1',
      uri: 'xmpp:robert@example.com',
      service: 'WhatsApp',
      user: 'bob',
      label: 'Personal',
      contexts: ['work'],
      pref: 1,
    },
  ]);
});
