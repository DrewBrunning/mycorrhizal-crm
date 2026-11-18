import type { ImportRowIssue } from '../api/import';

// Issue #2187: the skipped/failed rows of an import as a CSV the user can open
// next to the source file. Imported text is attacker-controlled, so a cell that
// would be read as a spreadsheet formula is prefixed with an apostrophe (the
// same neutralization the backend CSV export applies).
const FORMULA_START = /^[=+\-@\t\r]/;

function cell(value: string): string {
  const safe = FORMULA_START.test(value) ? `'${value}` : value;
  return /[",\r\n]/.test(safe) ? `"${safe.replace(/"/g, '""')}"` : safe;
}

export function importIssuesToCsv(issues: ImportRowIssue[]): string {
  // The Concept column (#2741) appears only when a warning carries one, so a
  // report of skipped/failed rows keeps its original six columns.
  const withConcept = issues.some((i) => i.concept);
  const header = ['Row', 'Name', 'Email', 'Phone', 'Status', 'Reason'];
  if (withConcept) header.push('Concept');
  const lines = [header.join(',')];
  for (const i of issues) {
    const fields = [
      String(i.row),
      cell(i.name),
      cell(i.email),
      cell(i.phone),
      i.status,
      cell(i.reason),
    ];
    if (withConcept) fields.push(cell(i.concept ?? ''));
    lines.push(fields.join(','));
  }
  return `${lines.join('\r\n')}\r\n`;
}
