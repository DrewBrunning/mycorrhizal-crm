// Accessible names for icon-only buttons in lists must say which item they act
// on (WCAG 2.4.6 / 2.5.3), but the item text can be a whole paragraph. This
// collapses whitespace and shortens it so a screen-reader's elements list stays
// scannable.
export const ITEM_LABEL_MAX = 40;

export function itemLabel(text: string | null | undefined, max: number = ITEM_LABEL_MAX): string {
  const flat = (text ?? '').replace(/\s+/g, ' ').trim();
  if (flat.length <= max) return flat;
  return `${flat.slice(0, max - 1).trimEnd()}…`;
}
