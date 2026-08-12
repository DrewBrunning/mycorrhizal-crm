package models

import "strings"

// PhoneKey reduces a phone number to a canonical comparison key: its digits,
// keeping at most the last 10. Two numbers with the same key are treated as
// the same number. Returns "" for anything with fewer than 7 digits so that
// short/extension-like values never collapse onto each other through a shared
// empty key.
//
// Lives here rather than in services because Contact.BeforeSave needs it to
// build the phones_normalized search column (T69), and models cannot import
// services. Callers in the services package use models.PhoneKey directly —
// there is deliberately only one implementation (T68).
func PhoneKey(phone string) string {
	digits := NormalizePhoneDigits(phone)
	if len(digits) < 7 {
		return ""
	}
	if len(digits) > 10 {
		return digits[len(digits)-10:]
	}
	return digits
}

// NormalizePhoneDigits strips everything but the ASCII digits from a phone
// number, preserving order. It is the lower-level building block behind
// PhoneKey and the phones_normalized search column (T69); it does not apply
// the last-10 truncation PhoneKey does.
func NormalizePhoneDigits(phone string) string {
	var normalized strings.Builder
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			normalized.WriteRune(r)
		}
	}
	return normalized.String()
}
