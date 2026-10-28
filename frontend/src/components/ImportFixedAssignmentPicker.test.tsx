import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { useState } from 'react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { createCircle, listCircles } from '../api/circles';
import { listTags } from '../api/tags';
import ImportFixedAssignmentPicker from './ImportFixedAssignmentPicker';

afterEach(cleanup);

vi.mock('../api/circles', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/circles')>()),
  listCircles: vi.fn(),
  createCircle: vi.fn(),
}));
vi.mock('../api/tags', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/tags')>()),
  listTags: vi.fn(),
}));

const circle = (id: string, name: string) => ({ id, name, created_at: '', updated_at: '' });

function Harness({
  onChange,
}: {
  onChange: (v: { circleIds: string[]; tagIds: string[] }) => void;
}) {
  const [v, setV] = useState({ circleIds: [] as string[], tagIds: [] as string[] });
  return (
    <ImportFixedAssignmentPicker
      {...v}
      onChange={(n) => {
        setV(n);
        onChange(n);
      }}
    />
  );
}

test('selects an existing circle', async () => {
  vi.mocked(listCircles).mockResolvedValue({
    circles: [circle('c-1', 'Family')],
    total: 1,
    next_cursor: '',
    limit: 200,
  });
  vi.mocked(listTags).mockResolvedValue({ tags: [], total: 0, next_cursor: '', limit: 200 });
  vi.mocked(createCircle).mockResolvedValue({ message: 'ok', circle: circle('c-2', 'Conf') });
  const onChange = vi.fn();
  render(<Harness onChange={onChange} />);

  const box = screen.getByRole('combobox', { name: 'Circles' });
  fireEvent.mouseDown(box);
  fireEvent.click(await screen.findByText('Family'));
  expect(onChange).toHaveBeenLastCalledWith({ circleIds: ['c-1'], tagIds: [] });
});

test('creates a new circle from typed text', async () => {
  vi.mocked(listCircles).mockResolvedValue({
    circles: [circle('c-1', 'Family')],
    total: 1,
    next_cursor: '',
    limit: 200,
  });
  vi.mocked(listTags).mockResolvedValue({ tags: [], total: 0, next_cursor: '', limit: 200 });
  vi.mocked(createCircle).mockResolvedValue({ message: 'ok', circle: circle('c-2', 'Conf') });
  const onChange = vi.fn();
  render(<Harness onChange={onChange} />);

  const box = screen.getByRole('combobox', { name: 'Circles' });
  fireEvent.focus(box);
  fireEvent.change(box, { target: { value: 'Conf' } });
  fireEvent.click(await screen.findByText('Create "Conf"'));
  await waitFor(() =>
    expect(onChange).toHaveBeenLastCalledWith({ circleIds: ['c-2'], tagIds: [] }),
  );
  expect(createCircle).toHaveBeenCalledWith('Conf');
});
