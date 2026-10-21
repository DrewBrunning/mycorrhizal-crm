import { afterEach, expect, test, vi } from 'vitest';
import { useTimeZone } from './timeZone';

afterEach(() => {
  vi.restoreAllMocks();
});

test('simulates a negative-offset zone on every local Date accessor', () => {
  const restore = useTimeZone('America/Chicago');
  // 22:30 on 11 March in Chicago is 03:30Z on 12 March.
  const instant = new Date('2026-03-12T03:30:00Z');

  expect(instant.getFullYear()).toBe(2026);
  expect(instant.getMonth()).toBe(2);
  expect(instant.getDate()).toBe(11);
  expect(instant.getDay()).toBe(3);
  expect(instant.getHours()).toBe(22);
  expect(instant.getMinutes()).toBe(30);
  expect(instant.getSeconds()).toBe(0);
  expect(instant.getTimezoneOffset()).toBe(300);

  const midnight = new Date('2026-03-12T03:30:00Z');
  midnight.setHours(0, 0, 0, 0);
  expect(midnight.toISOString()).toBe('2026-03-11T05:00:00.000Z');

  restore();
  expect(vi.isMockFunction(Date.prototype.getDate)).toBe(false);
});

test('simulates a positive-offset zone, including setHours defaults', () => {
  useTimeZone('Pacific/Auckland');
  // 08:00 on 12 June in Auckland is 20:00Z on 11 June.
  const instant = new Date('2026-06-11T20:00:00Z');

  expect(instant.getFullYear()).toBe(2026);
  expect(instant.getMonth()).toBe(5);
  expect(instant.getDate()).toBe(12);
  expect(instant.getDay()).toBe(5);
  expect(instant.getHours()).toBe(8);
  expect(instant.getMinutes()).toBe(0);
  expect(instant.getSeconds()).toBe(0);
  expect(instant.getTimezoneOffset()).toBe(-720);

  const hoursOnly = new Date('2026-06-11T20:00:00Z');
  hoursOnly.setHours(9);
  expect(hoursOnly.toISOString()).toBe('2026-06-11T21:00:00.000Z');

  const withMinutes = new Date('2026-06-11T20:00:00Z');
  withMinutes.setHours(9, 15);
  expect(withMinutes.toISOString()).toBe('2026-06-11T21:15:00.000Z');

  const withSeconds = new Date('2026-06-11T20:00:00Z');
  withSeconds.setHours(9, 15, 30);
  expect(withSeconds.toISOString()).toBe('2026-06-11T21:15:30.000Z');

  const withMs = new Date('2026-06-11T20:00:00Z');
  withMs.setHours(9, 15, 30, 500);
  expect(withMs.toISOString()).toBe('2026-06-11T21:15:30.500Z');
});

test('reuses a cached formatter for the same zone', () => {
  useTimeZone('America/Chicago');
  const first = new Date('2026-03-12T03:30:00Z').getHours();
  vi.restoreAllMocks();
  useTimeZone('America/Chicago');
  expect(new Date('2026-03-12T03:30:00Z').getHours()).toBe(first);
});
