import { test, expect, vi, afterEach } from 'vitest';
import { render, screen, cleanup, fireEvent } from '@testing-library/react';
import '../i18n/config';
import LifeEventDialog from './LifeEventDialog';

afterEach(cleanup);

function renderDialog(props: Partial<React.ComponentProps<typeof LifeEventDialog>> = {}) {
  const defaults: React.ComponentProps<typeof LifeEventDialog> = {
    open: true,
    onClose: vi.fn(),
    onSave: vi.fn().mockResolvedValue(undefined),
    ...props,
  };
  return render(<LifeEventDialog {...defaults} />);
}

async function chooseCategory(label: string) {
  fireEvent.mouseDown(screen.getByLabelText('Category *'));
  fireEvent.click(await screen.findByText(label));
}

async function chooseType(label: string) {
  fireEvent.mouseDown(screen.getByLabelText('Event Type *'));
  fireEvent.click(await screen.findByText(label));
}

test('create mode: the type field is disabled until a category is chosen', () => {
  renderDialog();
  expect(screen.getByLabelText('Category *')).toBeInTheDocument();
  expect(screen.getByLabelText('Event Type')).toBeDisabled();
});

test('choosing a category reveals that category\'s type options plus "Add a new life event type"', async () => {
  renderDialog();
  await chooseCategory('Home & Living');

  fireEvent.mouseDown(screen.getByLabelText('Event Type *'));
  expect(await screen.findByText('Moved')).toBeInTheDocument();
  expect(screen.getByText('Bought a home')).toBeInTheDocument();
  expect(screen.getByText('Add a new life event type')).toBeInTheDocument();
  // A different category's types must not leak into this list.
  expect(screen.queryByText('Got engaged')).not.toBeInTheDocument();
});

test('requires a category before saving', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave });

  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  expect(await screen.findByText('Category is required')).toBeInTheDocument();
  expect(onSave).not.toHaveBeenCalled();
});

test('requires a type after a category is chosen', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave });

  await chooseCategory('Health & Wellness');
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  expect(await screen.findByText('Event type is required')).toBeInTheDocument();
  expect(onSave).not.toHaveBeenCalled();
});

test('saves a predefined type together with its category', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave });

  await chooseCategory('Home & Living');
  await chooseType('Bought a home');
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await vi.waitFor(() => expect(onSave).toHaveBeenCalled());
  expect(onSave).toHaveBeenCalledWith(
    expect.objectContaining({ type: 'bought_a_home', category: 'home_living' })
  );
});

test('"Add a new life event type" reveals a free-text field and saves the custom value under the chosen category', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave });

  await chooseCategory('Health & Wellness');
  await chooseType('Add a new life event type');

  const customField = await screen.findByLabelText('Custom event name *');
  fireEvent.change(customField, { target: { value: 'Started a podcast' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await vi.waitFor(() => expect(onSave).toHaveBeenCalled());
  expect(onSave).toHaveBeenCalledWith(
    expect.objectContaining({ type: 'Started a podcast', category: 'health_wellness' })
  );
});

test('edit mode pre-fills category and type for a predefined event', () => {
  renderDialog({
    initial: { type: 'married', category: 'family_relationships' },
  });

  // MUI's menu-based Select renders its picked value as a div[role=combobox]
  // with the option's display text, not a native <select>/<input> — assert
  // on that text rather than .toHaveValue().
  expect(screen.getByLabelText('Category *')).toHaveTextContent('Family & Relationships');
  expect(screen.getByLabelText('Event Type *')).toHaveTextContent('Married');
});

test('edit mode falls back to the "Other / Uncategorized" bucket for a pre-migration event with no category, and shows its raw type as free text', () => {
  renderDialog({
    initial: { type: 'started a podcast' },
  });

  expect(screen.getByLabelText('Category *')).toHaveTextContent('Other / Uncategorized');
  // Uncategorized renders Type as a plain free-text TextField (real <input>).
  expect(screen.getByLabelText('Event Type *')).toHaveValue('started a podcast');
});

test('choosing "Other / Uncategorized" omits category from the saved payload', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave });

  await chooseCategory('Other / Uncategorized');
  const typeField = screen.getByLabelText('Event Type *');
  fireEvent.change(typeField, { target: { value: 'Some legacy event' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await vi.waitFor(() => expect(onSave).toHaveBeenCalled());
  expect(onSave).toHaveBeenCalledWith(
    expect.objectContaining({ type: 'Some legacy event', category: undefined })
  );
});
