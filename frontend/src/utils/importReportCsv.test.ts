import { expect, test } from 'vitest';
import { importIssuesToCsv } from './importReportCsv';

test('writes a header and one line per issue', () => {
  const csv = importIssuesToCsv([
    {
      row: 3,
      name: 'Ada Lovelace',
      email: 'ada@example.com',
      phone: '',
      status: 'skipped',
      reason: 'Skipped',
    },
  ]);
  expect(csv).toBe(
    'Row,Name,Email,Phone,Status,Reason\r\n3,Ada Lovelace,ada@example.com,,skipped,Skipped\r\n',
  );
});

test('quotes commas, quotes and newlines', () => {
  const csv = importIssuesToCsv([
    { row: 1, name: 'Lee, "Bo"', email: '', phone: '', status: 'failed', reason: 'a\nb' },
  ]);
  expect(csv).toContain('"Lee, ""Bo"""');
  expect(csv).toContain('"a\nb"');
});

test('neutralises spreadsheet formulas in imported text', () => {
  const csv = importIssuesToCsv([
    {
      row: 1,
      name: '=HYPERLINK("x")',
      email: '+1@x.com',
      phone: '@cmd',
      status: 'skipped',
      reason: '-1',
    },
  ]);
  const body = csv.split('\r\n')[1];
  expect(body).toContain("'=HYPERLINK");
  expect(body).toContain("'+1@x.com");
  expect(body).toContain("'@cmd");
  expect(body).toContain("'-1");
});

// #2741: a warning row (imported, but a date was dropped) is exported with
// status "warning" and its concept; reports without one keep the six columns.
test('adds a Concept column only when an issue carries a concept', () => {
  const csv = importIssuesToCsv([
    {
      row: 1,
      name: 'Ada',
      email: 'ada@example.com',
      phone: '',
      status: 'warning',
      concept: 'birthday',
      reason: 'birthday not imported: invalid value',
    },
    { row: 2, name: 'Bob', email: '', phone: '', status: 'skipped', reason: 'duplicate' },
  ]);
  expect(csv).toBe(
    'Row,Name,Email,Phone,Status,Reason,Concept\r\n' +
      '1,Ada,ada@example.com,,warning,birthday not imported: invalid value,birthday\r\n' +
      '2,Bob,,,skipped,duplicate,\r\n',
  );
});
