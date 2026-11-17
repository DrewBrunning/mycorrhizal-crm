/**
 * A readable fallback label for an open-vocabulary token that has no
 * translation ("gift" -> "Gift", "media_movie" -> "Media movie"). The backend
 * never rejects an unknown preference category or life-event type, so a legacy
 * or custom token must still read as words, not as a raw machine value (issue
 * #2681). Pass it as the i18next default value: `t(key, humanizeToken(token))`.
 */
export function humanizeToken(token: string): string {
  const words = token.replace(/[_-]+/g, ' ').trim();
  return words ? words.charAt(0).toUpperCase() + words.slice(1) : token;
}
