import { expect, test } from 'vitest';
import de from './locales/de.json';

// #2686: the German locale addresses the user with the formal "Sie" (maintainer
// decision, 2026-11-17). This fails on the informal pronoun set and on the
// informal imperatives the audit found, so a new "du" string cannot slip back
// in. Whole words only; "Sie/Ihr/Ihnen" are the formal forms and are not listed.
const INFORMAL_PRONOUNS =
  /(?<![\p{L}])(?:du|dich|dir|dein|deine|deinen|deinem|deiner|deines)(?![\p{L}])/iu;
const INFORMAL_IMPERATIVES =
  /(?<![\p{L}])(?:wähle|prüfe|erstelle|verwende|exportiere|füge|sende|setze|lege|verbinde|aktualisiere|verknüpfe|importiere|ändere|teile|erfasse|lasse|beschreibe|führe|aktiviere|bestätige|benenne|gib|bleibe|bleib|mach|hake|tippe|behalte|erlaube|schließe|öffne|lösche|entferne|nutze|speichere|erhöhe|lade|verfeinere|notiere|verwalte|widerrufe|filtere)(?![\p{L}])/iu;

function leaves(node: unknown, path = ''): Array<[string, string]> {
  if (typeof node === 'string') return [[path, node]];
  if (typeof node !== 'object' || node === null) return [];
  return Object.entries(node).flatMap(([k, v]) => leaves(v, path ? `${path}.${k}` : k));
}

const strings = leaves(de);

test('de.json addresses the user formally: no du/dein pronouns', () => {
  expect(strings.filter(([, v]) => INFORMAL_PRONOUNS.test(v)).map(([k]) => k)).toEqual([]);
});

test('de.json uses Sie imperatives, not informal ones', () => {
  expect(strings.filter(([, v]) => INFORMAL_IMPERATIVES.test(v)).map(([k]) => k)).toEqual([]);
});

test('the formal form is actually used', () => {
  expect(
    strings.filter(([, v]) => /(?<![\p{L}])(?:Sie|Ihr\p{L}*)(?![\p{L}])/u.test(v)).length,
  ).toBeGreaterThan(100);
});
