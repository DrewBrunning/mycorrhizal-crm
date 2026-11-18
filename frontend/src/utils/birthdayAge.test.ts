import { expect, test } from 'vitest';
import { upcomingBirthdayAge } from './birthdayAge';

// Issue #2681: the dashboard said "21.11. (41 years old)" for a birthday that
// had not happened yet. The age shown beside an upcoming date is the age they
// will turn.
const now = new Date(2026, 10, 17, 12, 0); // 17 Nov 2026

test('a birthday still ahead this year is the age they will turn', () => {
  expect(upcomingBirthdayAge(1985, '11', '21', now)).toEqual({ age: 41, isToday: false });
});

test('a birthday today is the age they now are', () => {
  expect(upcomingBirthdayAge(1985, '11', '17', now)).toEqual({ age: 41, isToday: true });
});

test('a birthday already passed this year rolls to next year', () => {
  expect(upcomingBirthdayAge(1985, '01', '05', now)).toEqual({ age: 42, isToday: false });
});

test('a leap-day birthday is compared as calendar digits', () => {
  expect(upcomingBirthdayAge(2000, '02', '29', now)).toEqual({ age: 27, isToday: false });
});
