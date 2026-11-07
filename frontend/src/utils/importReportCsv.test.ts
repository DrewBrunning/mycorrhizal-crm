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
