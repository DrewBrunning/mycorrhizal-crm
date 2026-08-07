package services

import (
	"fmt"
	"io"
	"mime/quotedprintable"
	"strings"

	"mycorrhizal/contactmodel"
)

// vcard21EncodingTokens are the legacy vCard 2.1 bare parameter values this
// normalizer recognizes as Content-Transfer-Encoding tokens (RFC 1521 §5,
// reused by vCard 2.1 for PHOTO/LOGO/SOUND/KEY inline binary and for
// QUOTED-PRINTABLE text). vCard 2.1's bare-token grammar has no third
// category: any other bare token is a TYPE value.
var vcard21EncodingTokens = map[string]bool{
	"BASE64":           true,
	"B":                true,
	"QUOTED-PRINTABLE": true,
	"7BIT":             true,
	"8BIT":             true,
}

// normalizeVCard21 rewrites a single vCard 2.1 BEGIN..END block's legacy
// bare-token parameter grammar (";CELL;PREF:", ";ENCODING=BASE64;JPEG:", ...)
// into the explicit TYPE=/ENCODING= form go-vcard's decoder (built for vCard
// 3.0/4.0) actually understands, and decodes any property that declares
// ENCODING=QUOTED-PRINTABLE (T50). It runs on the raw block bytes as a
// whole, once, before the block ever reaches go-vcard: the corruption T50
// fixes happens inside go-vcard's own decoder (see that ticket's
// investigation), not in either adapter's own logic, so patching either
// adapter after the fact can't fix it -- the bytes have to be fixed first.
//
// Every bare parameter token is classified generically -- a recognized
// encoding token (vcard21EncodingTokens) becomes ENCODING=<token>, anything
// else becomes TYPE=<token> -- regardless of position or how many other bare
// tokens share the property, so this isn't scoped to any one file's
// parameter ordering (T50's explicit trap).
func normalizeVCard21(block []byte) ([]byte, []contactmodel.Diagnostic) {
	lines := reconstructVCard21Lines(block)
	out := make([]string, 0, len(lines))
	var diags []contactmodel.Diagnostic
	for _, line := range lines {
		normalized, lineDiags := normalizeVCard21Line(line)
		out = append(out, normalized)
		diags = append(diags, lineDiags...)
	}
	return []byte(strings.Join(out, "\r\n") + "\r\n"), diags
}

// reconstructVCard21Lines splits a block into logical (fully unfolded)
// property lines. Two independent folding mechanisms are undone:
//
//   - standard RFC 2425 leading-whitespace continuation, so a property
//     split across physical lines by a normal fold is joined back into one;
//   - QUOTED-PRINTABLE's own soft-line-break continuation (RFC 2045 §6.7: a
//     trailing "=" with *no* leading whitespace on the next physical line),
//     for any property whose own parameter section declares
//     ENCODING=QUOTED-PRINTABLE -- go-vcard's decoder has no notion of this
//     second mechanism at all, so left alone it silently truncates the
//     property at the first soft break and drops the rest as an
//     unparseable stray line.
//
// Only a property's own initiating physical line is inspected for the
// ENCODING declaration -- real vCard 2.1 writers never split the ENCODING
// parameter itself across a fold.
func reconstructVCard21Lines(block []byte) []string {
	raw := strings.ReplaceAll(string(block), "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")
	rawLines := strings.Split(raw, "\n")

	var logical []string
	for i := 0; i < len(rawLines); i++ {
		line := rawLines[i]
		if line == "" {
			continue
		}
		if len(logical) > 0 && (line[0] == ' ' || line[0] == '\t') {
			logical[len(logical)-1] += line[1:]
			continue
		}
		if isQuotedPrintableParamLine(line) {
			for strings.HasSuffix(line, "=") && i+1 < len(rawLines) {
				i++
				line = strings.TrimSuffix(line, "=") + rawLines[i]
			}
		}
		logical = append(logical, line)
	}
	return logical
}

// isQuotedPrintableParamLine reports whether a property line's parameter
// section (the text before the first top-level ':') declares
// ENCODING=QUOTED-PRINTABLE, bare or explicit.
func isQuotedPrintableParamLine(line string) bool {
	params, _, ok := splitTopLevelColon(line)
	if !ok {
		return false
	}
	return strings.Contains(strings.ToUpper(params), "QUOTED-PRINTABLE")
}

// splitTopLevelColon splits s at its first ':' that isn't inside a
// double-quoted parameter value -- the boundary between a property's
// [group.]NAME[;params] section and its value.
func splitTopLevelColon(s string) (before, after string, ok bool) {
	inQuotes := false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			inQuotes = !inQuotes
		case ':':
			if !inQuotes {
				return s[:i], s[i+1:], true
			}
		}
	}
	return s, "", false
}

// splitTopLevelSemicolons splits s on ';' that aren't inside a
// double-quoted parameter value.
func splitTopLevelSemicolons(s string) []string {
	var out []string
	inQuotes := false
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			inQuotes = !inQuotes
		case ';':
			if !inQuotes {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	return append(out, s[start:])
}

// normalizeVCard21Line rewrites one already-unfolded logical property line:
// bare parameter tokens become TYPE=/ENCODING=, and a QUOTED-PRINTABLE value
// is decoded in place (mime/quotedprintable does the actual decode -- this
// only wires the value into it and escapes the result back into a single
// vCard text value).
func normalizeVCard21Line(line string) (string, []contactmodel.Diagnostic) {
	// Mirrors go-vcard's own parseGroup: only a literal '.' before the
	// first ';'/':' counts as a group prefix.
	group := ""
	rest := line
	if i := strings.IndexAny(line, ".;:"); i >= 0 && line[i] == '.' {
		group, rest = line[:i], line[i+1:]
	}

	i := strings.IndexAny(rest, ";:")
	if i < 0 {
		// Not a well-formed property line -- leave it for go-vcard's own
		// decoder to skip (it already tolerates and drops unparseable
		// lines rather than failing the whole card).
		return line, nil
	}
	name := rest[:i]
	if rest[i] != ';' {
		// No parameter section at all -- nothing for this normalizer to do.
		return line, nil
	}

	paramsSec, value, ok := splitTopLevelColon(rest[i+1:])
	if !ok {
		return line, nil
	}

	rawTokens := splitTopLevelSemicolons(paramsSec)
	tokens := make([]string, 0, len(rawTokens))
	for _, tok := range rawTokens {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		if strings.Contains(tok, "=") {
			tokens = append(tokens, tok)
			continue
		}
		if vcard21EncodingTokens[strings.ToUpper(tok)] {
			tokens = append(tokens, "ENCODING="+tok)
		} else {
			tokens = append(tokens, "TYPE="+tok)
		}
	}

	var diags []contactmodel.Diagnostic
	qpIdx := -1
	for idx, tok := range tokens {
		k, v, hasEq := strings.Cut(tok, "=")
		if hasEq && strings.EqualFold(strings.TrimSpace(k), "ENCODING") && strings.EqualFold(strings.TrimSpace(v), "QUOTED-PRINTABLE") {
			qpIdx = idx
			break
		}
	}
	if qpIdx >= 0 {
		decoded, err := io.ReadAll(quotedprintable.NewReader(strings.NewReader(value)))
		if err != nil {
			diags = append(diags, contactmodel.Diagnostic{
				Severity: "warn",
				Concept:  "vcard21.quoted-printable",
				Message:  fmt.Sprintf("%s: QUOTED-PRINTABLE value could not be decoded (%v); kept as raw encoded text", strings.ToUpper(name), err),
			})
		} else {
			value = escapeVCardValue(string(decoded))
			tokens = append(tokens[:qpIdx], tokens[qpIdx+1:]...)
		}
	}

	var b strings.Builder
	if group != "" {
		b.WriteString(group)
		b.WriteByte('.')
	}
	b.WriteString(name)
	for _, tok := range tokens {
		b.WriteByte(';')
		b.WriteString(tok)
	}
	b.WriteByte(':')
	b.WriteString(value)
	return b.String(), diags
}

// escapeVCardValue escapes a decoded QUOTED-PRINTABLE value so it survives
// as a single vCard text-value line. go-vcard's own value unescaper
// (backslash-backslash, backslash-n) is the exact inverse of this, so
// re-escaping here round-trips through it correctly. Backslashes are
// escaped first so the backslashes this introduces for newlines aren't
// themselves re-escaped.
func escapeVCardValue(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\r\n", "\\n")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\n")
	return s
}
