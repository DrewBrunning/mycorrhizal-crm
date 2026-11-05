import { afterEach, describe, expect, test, vi } from 'vitest';
import { errorEnvelope, mockApi, setupMswServer } from '../test/mswServer';
import { ApiError, isPreconditionFailed } from './client';
import {
  adoptContactImage,
  clearContactMainImage,
  getContactPhotos,
  patchContactImage,
  setContactMainImage,
  uploadContactImage,
} from './contactImages';

setupMswServer();
afterEach(() => vi.unstubAllGlobals());

describe('contact gallery API (ADR 0040 slice c)', () => {
  test('getContactPhotos GETs the aggregated /photos list', async () => {
    const body = { photos: [], total: 0 };
    const calls = mockApi('get', '/contacts/4/photos', body);
    expect(await getContactPhotos(4)).toEqual(body);
    expect(calls[0].url).toBe('/api/v1/contacts/4/photos');
  });

  test('setContactMainImage PUTs image_id with the quoted If-Match revision', async () => {
    const calls = mockApi('put', '/contacts/4/main-image', {
      message: 'ok',
      main_image_id: 9,
      revision: 8,
    });
    const out = await setContactMainImage(4, 9, 7);
    expect(out).toEqual({ message: 'ok', main_image_id: 9, revision: 8 });
    expect(calls[0].method).toBe('PUT');
    expect(calls[0].body).toEqual({ image_id: 9 });
    expect(calls[0].headers.get('if-match')).toBe('"7"');
  });

  test('setContactMainImage sends no If-Match when the revision is unknown', async () => {
    const calls = mockApi('put', '/contacts/4/main-image', { message: 'ok', revision: 2 });
    await setContactMainImage(4, 9);
    expect(calls[0].headers.get('if-match')).toBeNull();
  });

  test('a stale revision surfaces as a precondition failure', async () => {
    mockApi(
      'put',
      '/contacts/4/main-image',
      errorEnvelope(412, 'PRECONDITION_FAILED', 'stale revision'),
    );
    const err = await setContactMainImage(4, 9, 1).catch((e) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(isPreconditionFailed(err)).toBe(true);
  });

  test('clearContactMainImage DELETEs main-image with If-Match', async () => {
    const calls = mockApi('delete', '/contacts/4/main-image', { message: 'ok', revision: 9 });
    expect((await clearContactMainImage(4, 8)).revision).toBe(9);
    expect(calls[0].method).toBe('DELETE');
    expect(calls[0].headers.get('if-match')).toBe('"8"');
  });

  test('patchContactImage sends a crop rectangle, and an explicit null to clear it', async () => {
    const image = { id: 9 };
    const calls = mockApi('patch', '/contacts/4/images/9', { image });
    const crop = { x: 0.1, y: 0.2, w: 0.5, h: 0.5 };
    expect(await patchContactImage(4, 9, { crop })).toEqual(image);
    await patchContactImage(4, 9, { crop: null });
    expect(calls[0].body).toEqual({ crop });
    expect(calls[1].body).toEqual({ crop: null });
  });

  test('patchContactImage omits crop when only sensitivity changes', async () => {
    const calls = mockApi('patch', '/contacts/4/images/9', { image: { id: 9 } });
    await patchContactImage(4, 9, { sensitivity: 'private' });
    expect(calls[0].body).toEqual({ sensitivity: 'private' });
  });

  test('adoptContactImage POSTs the source and returns the image and dedupe flag', async () => {
    const calls = mockApi(
      'post',
      '/contacts/4/images/adopt',
      new Response(JSON.stringify({ image: { id: 12 }, deduplicated: true }), {
        status: 200,
        headers: { 'content-type': 'application/json' },
      }),
    );
    const out = await adoptContactImage(4, { source_kind: 'note', source_id: 3, image_id: 5 });
    expect(out).toEqual({ image: { id: 12 }, deduplicated: true });
    expect(calls[0].body).toEqual({ source_kind: 'note', source_id: 3, image_id: 5 });
  });

  // v1.6.0 review: the upload dropped the dHash dedupe flag, so re-uploading a
  // photo already in the gallery looked like it silently did nothing.
  test.each([
    [{ image: { id: 12 }, deduplicated: true }, true],
    [{ image: { id: 13 } }, false],
  ])(
    'uploadContactImage returns the image and the dedupe flag (%j)',
    async (body, deduplicated) => {
      vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce({ ok: true, json: async () => body }));
      const file = new File(['x'], 'photo.jpg', { type: 'image/jpeg' });
      expect(await uploadContactImage(4, file)).toEqual({ image: body.image, deduplicated });
    },
  );

  test('adoptContactImage maps the 409 image-cap error', async () => {
    mockApi('post', '/contacts/4/images/adopt', errorEnvelope(409, 'CONFLICT', 'gallery full'));
    const err = await adoptContactImage(4, {
      source_kind: 'activity',
      source_id: 1,
      image_id: 2,
    }).catch((e) => e);
    expect(err.status).toBe(409);
  });
});
