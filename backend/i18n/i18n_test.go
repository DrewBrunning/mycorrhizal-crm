package i18n

import "testing"

// TestNormalizeSupportedLanguage is the regression test for backlog item 12
// (docs/fork-plan/95-backlog-and-priorities.md): IsValidLanguage used to
// normalize via normalizeLanguage, which falls back to DefaultLanguage for
// any unrecognized input, so it returned true for literally any string.
// NormalizeSupportedLanguage must genuinely reject empty/unsupported input.
func TestNormalizeSupportedLanguage(t *testing.T) {
	cases := []struct {
		name     string
		lang     string
		wantNorm string
		wantOK   bool
	}{
		{"exact supported code", "de", "de", true},
		{"case-insensitive", "DE", "de", true},
		{"BCP-47 region subtag stripped", "en-US", "en", true},
		{"BCP-47 region subtag stripped, uppercase", "DE-AT", "de", true},
		{"empty input rejected", "", "", false},
		{"garbage rejected", "xx", "", false},
		{"garbage with region subtag rejected", "xx-YY", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotNorm, gotOK := NormalizeSupportedLanguage(tc.lang)
			if gotOK != tc.wantOK {
				t.Fatalf("NormalizeSupportedLanguage(%q) ok = %v, want %v", tc.lang, gotOK, tc.wantOK)
			}
			if gotNorm != tc.wantNorm {
				t.Fatalf("NormalizeSupportedLanguage(%q) = %q, want %q", tc.lang, gotNorm, tc.wantNorm)
			}
		})
	}
}

// TestIsValidLanguage_RejectsGarbage pins down the actual bug: before the
// fix, this returned true for any input at all.
func TestIsValidLanguage_RejectsGarbage(t *testing.T) {
	if IsValidLanguage("this-is-not-a-real-language-code") {
		t.Fatal("IsValidLanguage should reject an unrecognized code, not silently accept it as English")
	}
	if IsValidLanguage("") {
		t.Fatal("IsValidLanguage should reject an empty code")
	}
	if !IsValidLanguage("fr") {
		t.Fatal("IsValidLanguage should accept a genuinely supported code")
	}
}

// TestNormalizeLanguage_StillFallsBackForDisplayLookup confirms the
// unexported normalizeLanguage (used only by T()) keeps its own, deliberately
// different fallback-to-English behavior -- that one is correct for display
// lookups and was never the bug; only the validation path (IsValidLanguage/
// NormalizeSupportedLanguage) needed to stop reusing it.
func TestNormalizeLanguage_StillFallsBackForDisplayLookup(t *testing.T) {
	if got := normalizeLanguage("this-is-not-a-real-language-code"); got != DefaultLanguage {
		t.Fatalf("normalizeLanguage(garbage) = %q, want fallback %q", got, DefaultLanguage)
	}
	if got := normalizeLanguage(""); got != DefaultLanguage {
		t.Fatalf("normalizeLanguage(\"\") = %q, want fallback %q", got, DefaultLanguage)
	}
}

// --- Translation tests (require Init) ---

func initForTest(t *testing.T) {
	t.Helper()
	if !initialized {
		if err := Init(); err != nil {
			t.Fatalf("Init failed: %v", err)
		}
	}
}

func TestInit_LoadsTranslations(t *testing.T) {
	initForTest(t)

	translationsMu.RLock()
	defer translationsMu.RUnlock()

	if len(translations) != len(SupportedLanguages) {
		t.Fatalf("translations has %d languages, want %d", len(translations), len(SupportedLanguages))
	}
	if _, ok := translations["en"]; !ok {
		t.Fatal("English translations not loaded")
	}
	if _, ok := translations["de"]; !ok {
		t.Fatal("German translations not loaded")
	}
}

func TestInit_Idempotent(t *testing.T) {
	initForTest(t)
	if err := Init(); err != nil {
		t.Fatalf("second Init() returned error: %v", err)
	}
}

func TestT_ExactLanguageLookup(t *testing.T) {
	initForTest(t)

	if got := T("en", "email.reminder.subject"); got != "Your Daily Reminders" {
		t.Errorf("T(en, email.reminder.subject) = %q, want 'Your Daily Reminders'", got)
	}
	if got := T("en", "calendar.untitledEvent"); got != "Untitled calendar event" {
		t.Errorf("T(en, calendar.untitledEvent) = %q, want 'Untitled calendar event'", got)
	}
}

func TestT_FallbackToEnglish(t *testing.T) {
	initForTest(t)

	// German has its own translations, but requesting with a made-up language
	// should fall back to English.
	if got := T("xx", "email.reminder.subject"); got != "Your Daily Reminders" {
		t.Errorf("T(xx, ...) = %q, want English fallback 'Your Daily Reminders'", got)
	}

	// Empty language should also fall back to English.
	if got := T("", "email.reminder.subject"); got != "Your Daily Reminders" {
		t.Errorf("T('', ...) = %q, want English fallback 'Your Daily Reminders'", got)
	}
}

func TestT_ReturnsKeyWhenNotFound(t *testing.T) {
	initForTest(t)

	// Neither English nor any other language has this key — should return the
	// key itself as the last-resort value.
	if got := T("en", "nonexistent.key.path"); got != "nonexistent.key.path" {
		t.Errorf("T(en, nonexistent) = %q, want key itself", got)
	}

	// German has its own translation for email.reminder.subject (not a
	// fallback-to-English case — the key genuinely exists in German).
	if got := T("de", "email.reminder.subject"); got == "" {
		t.Errorf("T(de, email.reminder.subject) returned empty; German should have its own string")
	}
}

func TestT_Interpolation(t *testing.T) {
	initForTest(t)

	// email.reminder.inDays is "In {{days}} days"
	got := T("en", "email.reminder.inDays", map[string]string{"days": "3"})
	if got != "In 3 days" {
		t.Errorf("T(en, email.reminder.inDays, days=3) = %q, want 'In 3 days'", got)
	}

	// No params — should return the raw template.
	got = T("en", "email.reminder.inDays")
	if got != "In {{days}} days" {
		t.Errorf("T(en, email.reminder.inDays) no params = %q, want 'In {{days}} days'", got)
	}
}

func TestT_BCP47Normalization(t *testing.T) {
	initForTest(t)

	// "en-US" should normalize to "en" and find the English translation.
	if got := T("en-US", "email.reminder.subject"); got != "Your Daily Reminders" {
		t.Errorf("T(en-US, ...) = %q, want 'Your Daily Reminders'", got)
	}

	// "de-DE" should normalize to "de" and return the German translation.
	if got := T("de-DE", "email.reminder.subject"); got == "" || got == "email.reminder.subject" {
		t.Errorf("T(de-DE, ...) = %q, want a German translation", got)
	}
}

func TestInterpolate_NoParams(t *testing.T) {
	if got := interpolate("hello", nil...); got != "hello" {
		t.Errorf("interpolate with nil params = %q, want 'hello'", got)
	}
	if got := interpolate("hello"); got != "hello" {
		t.Errorf("interpolate with no params = %q, want 'hello'", got)
	}
}

func TestInterpolate_EmptyParamsMap(t *testing.T) {
	if got := interpolate("hello", map[string]string{}); got != "hello" {
		t.Errorf("interpolate with empty map = %q, want 'hello'", got)
	}
}

func TestLookup_IntermediateNodeIsNotAMap(t *testing.T) {
	initForTest(t)

	// "email.footer" is a string, so "email.footer.nonexistent" should hit
	// the "not a map" branch and return "".
	if got := lookup("en", "email.footer.nonexistent"); got != "" {
		t.Errorf("lookup(intermediate non-map) = %q, want ''", got)
	}
}

func TestLookup_FinalNodeIsNotAString(t *testing.T) {
	initForTest(t)

	// "email.reminder" is a map, not a string — lookup should return "".
	if got := lookup("en", "email.reminder"); got != "" {
		t.Errorf("lookup(non-string leaf) = %q, want ''", got)
	}
}

func TestLookup_UnknownLanguage(t *testing.T) {
	initForTest(t)

	if got := lookup("xx", "email.reminder.subject"); got != "" {
		t.Errorf("lookup(unknown lang) = %q, want ''", got)
	}
}

func TestT_AllSupportedLanguagesHaveExpectedKeys(t *testing.T) {
	initForTest(t)

	keys := []string{
		"email.footer",
		"email.reminder.subject",
		"email.reminder.inDays",
		"email.passwordReset.subject",
		"calendar.untitledEvent",
	}

	for _, lang := range SupportedLanguages {
		for _, key := range keys {
			result := T(lang, key)
			if result == key {
				t.Errorf("missing key %q in language %q", key, lang)
			}
		}
	}
}
