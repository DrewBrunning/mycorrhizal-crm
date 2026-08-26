import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, expect, test } from 'vitest';
import '../i18n/config';
import type { Card } from '../api/contacts';
import RelatedToMembersSection, { hasRelatedToOrMembers } from './RelatedToMembersSection';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(cleanup);

function card(overrides: Partial<Card> = {}): Card {
  return { ...overrides };
}

test('renders related entities with their relation list', () => {
  render(
    <RelatedToMembersSection
      card={card({
        relatedTo: [
          { target: 'https://example.com/urn:uuid:alice', relations: ['friend', 'colleague'] },
        ],
      })}
    />,
  );

  expect(screen.getByText('Related Entities')).toBeInTheDocument();
  expect(screen.getByText('Related to')).toBeInTheDocument();
  expect(
    screen.getByText('https://example.com/urn:uuid:alice (friend, colleague)'),
  ).toBeInTheDocument();
});

test('a related entity without relations renders without parentheses', () => {
  render(
    <RelatedToMembersSection
      card={card({ relatedTo: [{ target: 'https://example.com/urn:uuid:bob' }] })}
    />,
  );

  expect(screen.getByText('https://example.com/urn:uuid:bob')).toBeInTheDocument();
});

test('renders members under the Members caption', () => {
  render(
    <RelatedToMembersSection
      card={card({
        members: ['https://example.com/urn:uuid:carol', 'https://example.com/urn:uuid:dave'],
      })}
    />,
  );

  expect(screen.getByText('Members')).toBeInTheDocument();
  expect(screen.getByText('https://example.com/urn:uuid:carol')).toBeInTheDocument();
  expect(screen.getByText('https://example.com/urn:uuid:dave')).toBeInTheDocument();
});

test('renders nothing when the card has neither related entities nor members', () => {
  const { container } = render(<RelatedToMembersSection card={card()} />);

  expect(container).toBeEmptyDOMElement();
});

test('hasRelatedToOrMembers reports what the section will render', () => {
  expect(hasRelatedToOrMembers(card())).toBe(false);
  expect(hasRelatedToOrMembers(card({ relatedTo: [{ target: 'x' }] }))).toBe(true);
  expect(hasRelatedToOrMembers(card({ members: ['y'] }))).toBe(true);
  expect(hasRelatedToOrMembers(card({ relatedTo: [], members: [] }))).toBe(false);
});
