import { expect, test } from 'vitest';
import type { FieldDefinition } from '../api/fieldDefinitions';
import {
  distinctGroups,
  groupDefinitions,
  groupLength,
  hasAnyGroup,
  isGroupFilled,
} from './customFieldGroups';

function def(id: string, group: string): FieldDefinition {
  return {
    id,
    label: id,
    key: id,
    target: 'contact',
    type: 'string',
    projection: 'internal-only',
    sensitivity: 'normal',
    position: 0,
    group,
    created_at: '',
    updated_at: '',
  };
}

test('named groups sort alphabetically, ungrouped last, position order kept within a group', () => {
  const groups = groupDefinitions([
    def('a1', ''),
    def('z1', 'Work'),
    def('b1', 'Travel'),
    def('z2', 'Work'),
    def('a2', ''),
    def('b2', 'Travel'),
  ]);
  expect(groups.map((g) => g.group)).toEqual(['Travel', 'Work', '']);
  expect(groups[1].definitions.map((d) => d.id)).toEqual(['z1', 'z2']);
  expect(groups[2].definitions.map((d) => d.id)).toEqual(['a1', 'a2']);
});

test('buckets are case-sensitive exact matches', () => {
  const groups = groupDefinitions([def('a', 'Travel'), def('b', 'travel')]);
  expect(groups).toHaveLength(2);
});

test('sorting is locale-aware', () => {
  const groups = groupDefinitions([def('a', 'Zebra'), def('b', 'Ärger')], 'de');
  expect(groups.map((g) => g.group)).toEqual(['Ärger', 'Zebra']);
});

test('no definitions gives no groups, and only populated buckets appear', () => {
  expect(groupDefinitions([])).toEqual([]);
  expect(groupDefinitions([def('a', 'X')]).map((g) => g.group)).toEqual(['X']);
});

test('hasAnyGroup and distinctGroups', () => {
  expect(hasAnyGroup([def('a', '')])).toBe(false);
  expect(hasAnyGroup([def('a', ''), def('b', 'X')])).toBe(true);
  expect(distinctGroups([def('a', 'B'), def('b', 'A'), def('c', 'B'), def('d', '')])).toEqual([
    'A',
    'B',
  ]);
});

test('groupLength counts characters, not UTF-16 units', () => {
  expect(groupLength('ab')).toBe(2);
  expect(groupLength('😀😀')).toBe(2);
});

test('isGroupFilled ignores empty values', () => {
  const g = { group: 'X', definitions: [def('a', 'X'), def('b', 'X')] };
  expect(isGroupFilled(g, undefined)).toBe(false);
  expect(
    isGroupFilled(
      g,
      new Map<string, unknown>([
        ['a', ''],
        ['b', []],
      ]),
    ),
  ).toBe(false);
  expect(isGroupFilled(g, new Map<string, unknown>([['a', null]]))).toBe(false);
  expect(isGroupFilled(g, new Map<string, unknown>([['b', 'hi']]))).toBe(true);
  expect(isGroupFilled(g, new Map<string, unknown>([['b', false]]))).toBe(true);
});
