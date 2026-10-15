// Minimal in-memory CacheStorage for the share-target tests (jsdom has none).
export function makeFakeCaches(): CacheStorage {
  const stores = new Map<string, Map<string, string>>();
  const open = async (name: string) => {
    if (!stores.has(name)) stores.set(name, new Map());
    const store = stores.get(name) as Map<string, string>;
    return {
      put: async (url: string, res: Response) => {
        store.set(url, await res.text());
      },
      match: async (url: string) => (store.has(url) ? new Response(store.get(url)) : undefined),
      delete: async (req: string | Request) =>
        store.delete(typeof req === 'string' ? req : req.url),
      keys: async () => [...store.keys()].map((u) => new Request(u)),
    };
  };
  return { open } as unknown as CacheStorage;
}
