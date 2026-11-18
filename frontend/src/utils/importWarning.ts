// The concepts a row-level import warning can name (the stable token the
// backend sends as `concept` on a preview diagnostic and on a 'warning' report
// issue, issues #2653 and #2741). The text is localized by concept, never by
// the English server message; an unknown concept falls back to that message.
const WARNING_CONCEPTS = ['birthday', 'anniversary'] as const;

// The i18n key for a known warning concept, or undefined for any other.
export function importWarningKey(concept: string | undefined): string | undefined {
  return concept && (WARNING_CONCEPTS as readonly string[]).includes(concept)
    ? `contacts.import.warning.${concept}`
    : undefined;
}
