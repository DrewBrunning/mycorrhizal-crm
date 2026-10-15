import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  contactsToVCard,
  isContactPickerSupported,
  type PickedContact,
  pickContactsAsVCardFile,
} from './contactPicker';

afterEach(() => {
  vi.unstubAllGlobals();
  Reflect.deleteProperty(navigator, 'contacts');
  Reflect.deleteProperty(window, 'ContactsManager');
});

function stubPicker(
  select: (props: string[], opts?: unknown) => Promise<PickedContact[]>,
  supported = ['name', 'email', 'tel', 'address', 'icon'],
) {
  Object.defineProperty(navigator, 'contacts', {
    configurable: true,
    value: { getProperties: async () => supported, select },
  });
  vi.stubGlobal('ContactsManager', class {});
}

describe('contactsToVCard', () => {
  it('maps name, email, tel and address', async () => {
    const vcf = await contactsToVCard([
      {
        name: ['Ada Byron King'],
        email: ['ada@x.test'],
        tel: ['+1 555 0100'],
        address: [
          {
            addressLine: ['1 Main St', 'Apt 2'],
            city: 'Madison',
            region: 'WI',
            postalCode: '53703',
            country: 'US',
          },
        ],
      },
    ]);
    expect(vcf).toBe(
      [
        'BEGIN:VCARD',
        'VERSION:3.0',
        'FN:Ada Byron King',
        'N:King;Ada Byron;;;',
        'EMAIL:ada@x.test',
        'TEL:+1 555 0100',
        'ADR:;;1 Main St\\, Apt 2;Madison;WI;53703;US',
        'END:VCARD',
        '',
      ].join('\r\n'),
    );
  });

  it('escapes text and handles a single-word name', async () => {
    const vcf = await contactsToVCard([
      { name: [String.raw`Cher;\x`], address: [{ addressLine: ['a\nb'] }] },
    ]);
    expect(vcf).toContain(String.raw`FN:Cher\;\\x`);
    expect(vcf).toContain(String.raw`N:;Cher\;\\x;;;`);
    expect(vcf).toContain(String.raw`ADR:;;a\nb;;;;`);
  });

  it('falls back to email or phone for the display name and skips empty contacts', async () => {
    const vcf = await contactsToVCard([
      { email: ['only@x.test'] },
      { tel: ['555'] },
      { name: ['  '], email: [' '] },
      {},
    ]);
    expect(vcf).toContain('FN:only@x.test');
    expect(vcf).toContain('FN:555');
    expect(vcf.match(/BEGIN:VCARD/g)).toHaveLength(2);
    expect(await contactsToVCard([])).toBe('');
  });

  it('embeds the icon as a base64 photo and ignores empty or typeless ones', async () => {
    const png = new Blob([new Uint8Array([1, 2, 3])], { type: 'image/png' });
    const odd = new Blob([new Uint8Array([4])], { type: 'application/x-weird' });
    const empty = new Blob([], { type: 'image/png' });
    const vcf = await contactsToVCard([
      { name: ['A B'], icon: [png] },
      { name: ['C D'], icon: [odd] },
      { name: ['E F'], icon: [empty] },
    ]);
    expect(vcf).toContain('PHOTO;ENCODING=b;TYPE=PNG:AQID');
    expect(vcf).toContain('PHOTO;ENCODING=b;TYPE=JPEG:BA==');
    expect(vcf.match(/PHOTO/g)).toHaveLength(2);
  });

  it('dedupes phones by digits and emails by case', async () => {
    const vcf = await contactsToVCard([
      {
        name: ['A B'],
        tel: ['262-909-6849', '2629096849', '(262) 909 6849', '+1 555 0100'],
        email: ['A@x.test', 'a@X.test'],
      },
    ]);
    expect(vcf.match(/^TEL:/gm)).toHaveLength(2);
    expect(vcf).toContain('TEL:262-909-6849');
    expect(vcf.match(/^EMAIL:/gm)).toHaveLength(1);
  });

  it('strips the locality Chrome Android repeats in addressLine', async () => {
    const vcf = await contactsToVCard([
      {
        name: ['A B'],
        address: [
          {
            addressLine: ['451 Oak Grove Ave, South Beloit, IL 61080, USA'],
            city: 'South Beloit',
            region: 'IL',
            postalCode: '61080',
            country: 'USA',
          },
        ],
      },
    ]);
    expect(vcf).toContain('ADR:;;451 Oak Grove Ave;South Beloit;IL;61080;USA');
  });

  it('keeps a street line that is only the locality', async () => {
    const vcf = await contactsToVCard([
      { name: ['A B'], address: [{ addressLine: ['Madison'], city: 'Madison' }] },
    ]);
    expect(vcf).toContain('ADR:;;Madison;Madison;;;');
  });

  it('drops address entries with no content', async () => {
    const vcf = await contactsToVCard([{ name: ['A B'], address: [{ addressLine: [''] }] }]);
    expect(vcf).not.toContain('ADR');
  });
});

describe('pickContactsAsVCardFile', () => {
  it('is unsupported (and null) without the Contact Picker', async () => {
    expect(isContactPickerSupported()).toBe(false);
    expect(await pickContactsAsVCardFile()).toBeNull();
  });

  it('requests only supported properties and returns a .vcf file', async () => {
    const select = vi.fn().mockResolvedValue([{ name: ['A B'] }]);
    stubPicker(select, ['name', 'email']);
    expect(isContactPickerSupported()).toBe(true);
    const file = await pickContactsAsVCardFile();
    expect(select).toHaveBeenCalledWith(['name', 'email'], { multiple: true });
    expect(file?.name).toBe('device-contacts.vcf');
    expect(await file?.text()).toContain('FN:A B');
  });

  it('returns null when nothing usable was picked or the picker was dismissed', async () => {
    stubPicker(async () => []);
    expect(await pickContactsAsVCardFile()).toBeNull();
    stubPicker(async () => {
      throw new DOMException('dismissed', 'AbortError');
    });
    expect(await pickContactsAsVCardFile()).toBeNull();
  });

  it('rethrows other picker failures', async () => {
    stubPicker(async () => {
      throw new DOMException('no', 'SecurityError');
    });
    await expect(pickContactsAsVCardFile()).rejects.toThrow('no');
  });
});

describe('streetLine edge cases', () => {
  it('only strips whole trailing components', async () => {
    const vcf = await contactsToVCard([
      { name: ['A B'], address: [{ addressLine: ['12 Westmadison'], city: 'Madison' }] },
    ]);
    expect(vcf).toContain('ADR:;;12 Westmadison;Madison;;;');
  });
});
