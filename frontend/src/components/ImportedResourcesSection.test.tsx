import { test, expect, afterEach } from 'vitest';
import { render, screen, cleanup } from '@testing-library/react';
import '../i18n/config';
import ImportedResourcesSection from './ImportedResourcesSection';
import RelatedToMembersSection from './RelatedToMembersSection';
import { Card } from '../api/contacts';

afterEach(cleanup);

test('renders imported calendar, directory and contact-uri resources read-only (WP7)', () => {
  const card: Card = {
    calendars: [{ uri: 'https://cal.example.com/ada.ics' }],
    directories: [{ uri: 'https://dir.example.com/ada', kind: 'entry' }],
    contactUris: [{ uri: 'https://example.com/ada', mediaType: 'text/html' }],
  };
  render(<ImportedResourcesSection card={card} />);
  expect(screen.getByText('Imported Resources')).toBeInTheDocument();
  expect(screen.getByText(/https:\/\/cal\.example\.com\/ada\.ics/)).toBeInTheDocument();
  expect(screen.getByText(/https:\/\/dir\.example\.com\/ada/)).toBeInTheDocument();
  expect(screen.getByText(/https:\/\/example\.com\/ada/)).toBeInTheDocument();
});

test('renders nothing when no resources exist', () => {
  render(<ImportedResourcesSection card={{}} />);
  expect(screen.queryByText('Imported Resources')).toBeNull();
});

test('renders relatedTo and members read-only (WP8)', () => {
  const card: Card = {
    relatedTo: [{ target: 'uid:other-contact', relations: ['friend'] }],
    members: ['uid:member-a', 'uid:member-b'],
  };
  render(<RelatedToMembersSection card={card} />);
  expect(screen.getByText('Related Entities')).toBeInTheDocument();
  expect(screen.getByText('uid:other-contact (friend)')).toBeInTheDocument();
  expect(screen.getByText('uid:member-a')).toBeInTheDocument();
  expect(screen.getByText('uid:member-b')).toBeInTheDocument();
});
