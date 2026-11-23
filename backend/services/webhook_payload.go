package services

// Issue #2888 (v1.7.0 half of #2071): a webhook whose include_sensitive is
// explicitly false receives contact payloads without any above-normal
// (private/secret) data. The default stays full fidelity in v1.x, so nil and
// explicit true are unchanged; v2.0.0 flips the nil default (#2071).
//
// The filter is deliberately applied to the serialized envelope rather than to
// every struct field: any object that carries `sensitivity: private|secret`
// is removed, so a sensitivity-classified field added to an event payload
// later is dropped without a second allowlist. The one thing JSON cannot see
// is models.Contact's flat `address` scalar, which is the denormalized copy of
// the primary address and carries no sensitivity marker of its own — it is
// recomputed after the address list is filtered.

import (
	"bytes"
	"encoding/json"

	"mycorrhizal/models"
)

// asWebhookContact extracts a Contact from the value a handler passed to
// TriggerWebhooks. Handlers pass either a value (contact_write.go,
// trash_controller.go) or a pointer.
func asWebhookContact(data interface{}) (models.Contact, bool) {
	switch contact := data.(type) {
	case models.Contact:
		return contact, true
	case *models.Contact:
		if contact == nil {
			return models.Contact{}, false
		}
		return *contact, true
	default:
		return models.Contact{}, false
	}
}

// contactWithoutSensitiveAddresses drops private/secret addresses (and with
// them their coordinates) and recomputes the flat primary-address scalar, so a
// filtered payload cannot leak a sensitive primary through `address`. It
// returns the input unchanged when nothing was dropped, preserving nil vs []
// and avoiding needless work.
func contactWithoutSensitiveAddresses(contact models.Contact) models.Contact {
	kept := make([]models.ContactAddress, 0, len(contact.Addresses))
	dropped := false
	for _, address := range contact.Addresses {
		if models.IsSensitiveSensitivity(address.Sensitivity) {
			dropped = true
			continue
		}
		kept = append(kept, address)
	}
	if !dropped {
		return contact
	}

	contact.Addresses = kept
	// models.Contact.deriveDenormalized sets Address = FormatAddress(Addresses[0]).
	if len(kept) > 0 {
		contact.Address = models.FormatAddress(kept[0])
	} else {
		contact.Address = ""
	}
	return contact
}

// redactSensitiveJSON walks a decoded JSON value and removes every object
// whose `sensitivity` is private or secret, and every object nested inside a
// removed one. keep=false tells the parent to drop the value; a JSON null is
// a legitimate kept value, so the two are distinct.
func redactSensitiveJSON(value interface{}) (redacted interface{}, keep bool) {
	switch v := value.(type) {
	case map[string]interface{}:
		if sensitivity, ok := v["sensitivity"].(string); ok && models.IsSensitiveSensitivity(sensitivity) {
			return nil, false
		}
		for key, child := range v {
			if cleaned, keepChild := redactSensitiveJSON(child); keepChild {
				v[key] = cleaned
			} else {
				delete(v, key)
			}
		}
		return v, true
	case []interface{}:
		cleaned := make([]interface{}, 0, len(v))
		for _, child := range v {
			if kept, keepChild := redactSensitiveJSON(child); keepChild {
				cleaned = append(cleaned, kept)
			}
		}
		return cleaned, true
	default:
		return value, true
	}
}

// buildFilteredPayloadBody is buildPayloadBody with above-normal data removed.
// It is only used for a webhook that explicitly set include_sensitive=false;
// the caller must not fall back to the unfiltered body if this fails.
func buildFilteredPayloadBody(eventType string, data interface{}) ([]byte, error) {
	if contact, ok := asWebhookContact(data); ok {
		data = contactWithoutSensitiveAddresses(contact)
	}

	body, err := buildPayloadBody(eventType, data)
	if err != nil {
		return nil, err
	}

	// UseNumber keeps identifiers byte-identical instead of round-tripping
	// them through float64.
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var envelope interface{}
	if err := decoder.Decode(&envelope); err != nil {
		return nil, err
	}
	redacted, _ := redactSensitiveJSON(envelope)
	return json.Marshal(redacted)
}
