import { afterEach, expect, test, vi } from 'vitest';
import { getGraph } from './graph';

afterEach(() => {
  vi.unstubAllGlobals();
});

function stub() {
  const fetchMock = vi.fn().mockResolvedValue({
    ok: true,
    json: async () => ({ nodes: [], edges: [] }),
  });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

// ADR 0043 decision 2: entity nodes are strictly opt-in, so the default
// request is byte-for-byte what it was before entities existed.
test('the default request does not ask for entities', async () => {
  const fetchMock = stub();
  await getGraph();
  expect(fetchMock.mock.calls[0][0]).toMatch(/\/graph\?full=false$/);
});

test('includeEntities adds include_entities=true to the ego or full request', async () => {
  let fetchMock = stub();
  await getGraph({ includeEntities: true });
  expect(fetchMock.mock.calls[0][0]).toMatch(/\/graph\?full=false&include_entities=true$/);
  fetchMock = stub();
  await getGraph({ full: true, includeEntities: true });
  expect(fetchMock.mock.calls[0][0]).toMatch(/\/graph\?full=true&include_entities=true$/);
});
