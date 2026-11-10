import { cleanup, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { afterEach, expect, test } from 'vitest';
import '../i18n/config';
import type { ContactEntityLink } from '../api/entities';
import ProjectedFromChip from './ProjectedFromChip';

afterEach(cleanup);

const links = [{ id: 'l1', entity_id: 'e1', entity_name: 'Acme' } as ContactEntityLink];

function renderChip(elementId: string | undefined, entityLinks?: ContactEntityLink[]) {
  return render(
    <MemoryRouter>
      <ProjectedFromChip elementId={elementId} entityLinks={entityLinks} />
    </MemoryRouter>,
  );
}

test('names the source entity and links to its page', () => {
  renderChip('ent-l1', links);
  const chip = screen.getByRole('link', { name: 'from Acme' });
  expect(chip).toHaveAttribute('href', '/entities/e1');
});

test('falls back to a generic label while the links are unknown', () => {
  renderChip('ent-zzz', links);
  expect(screen.getByText('from a linked entity')).toBeInTheDocument();
  cleanup();
  renderChip('ent-l1', undefined);
  expect(screen.getByText('from a linked entity')).toBeInTheDocument();
});

test('renders nothing for a contact’s own (non-projected) value', () => {
  const { container } = renderChip('own-1', links);
  expect(container).toBeEmptyDOMElement();
  cleanup();
  const again = renderChip(undefined, links);
  expect(again.container).toBeEmptyDOMElement();
});
