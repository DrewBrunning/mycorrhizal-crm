package atrest

import (
	"context"
	"encoding/json"
	"reflect"

	"gorm.io/gorm/schema"
)

// GORM serializer integration. Two serializers are registered:
//
//   - "encrypted": for plain string columns (HowWeMet, life_events.description,
//     reminders.message, gifts.notes, audit before_snapshot, sync-conflict
//     values, ...). Value encrypts, Scan decrypts.
//   - "encryptedjson": for struct/JSON columns (contacts.card/crm/passthrough,
//     the neutral model). Value json-marshals then encrypts; Scan decrypts
//     then json-unmarshals, matching the built-in "json" serializer's NULL and
//     "null" handling exactly so the schema change is transparent to every
//     existing caller.
//
// Both run on every GORM read and write path, including db.Raw().Scan() into
// model-shaped structs (the search service does this) — GORM resolves the
// serializer by name at schema-parse time and applies it in field.Set. When
// the layer is not armed (tests, pre-key deployments), Encrypt/Decrypt pass
// values through unchanged, so existing behavior is byte-identical.

func init() {
	schema.RegisterSerializer("encrypted", encryptedSerializer{})
	schema.RegisterSerializer("encryptedjson", encryptedJSONSerializer{})
}

// encryptedSerializer encrypts a plain string column at rest.
type encryptedSerializer struct{}

func (encryptedSerializer) Scan(ctx context.Context, field *schema.Field, dst reflect.Value, dbValue interface{}) error {
	plain, err := Decrypt(dbString(dbValue))
	if err != nil {
		return err
	}
	return field.Set(ctx, dst, plain)
}

func (encryptedSerializer) Value(ctx context.Context, field *schema.Field, dst reflect.Value, fieldValue interface{}) (interface{}, error) {
	s, _ := fieldValue.(string)
	return Encrypt(s)
}

// encryptedJSONSerializer is the encrypted analogue of GORM's built-in json
// serializer: it JSON-encodes the struct on write, encrypts the bytes, and on
// read decrypts then decodes back into the struct. It reproduces the built-in
// serializer's NULL/"null" behavior so a struct field keeps its previous
// storage shape (NULL for a zero value) once encrypted.
type encryptedJSONSerializer struct{}

func (encryptedJSONSerializer) Scan(ctx context.Context, field *schema.Field, dst reflect.Value, dbValue interface{}) (err error) {
	fieldValue := reflect.New(field.FieldType)

	if dbValue != nil {
		var bytes []byte
		switch v := dbValue.(type) {
		case []byte:
			bytes = v
		case string:
			bytes = []byte(v)
		default:
			bytes, err = json.Marshal(v)
			if err != nil {
				return err
			}
		}

		if len(bytes) > 0 {
			plain, derr := Decrypt(string(bytes))
			if derr != nil {
				return derr
			}
			if plain == "" {
				// Encrypted empty string stores as "" — treat as zero value.
				field.ReflectValueOf(ctx, dst).Set(fieldValue.Elem())
				return nil
			}
			if err = json.Unmarshal([]byte(plain), fieldValue.Interface()); err != nil {
				return err
			}
		}
	}

	field.ReflectValueOf(ctx, dst).Set(fieldValue.Elem())
	return
}

func (encryptedJSONSerializer) Value(ctx context.Context, field *schema.Field, dst reflect.Value, fieldValue interface{}) (interface{}, error) {
	result, err := json.Marshal(fieldValue)
	if err != nil {
		return nil, err
	}
	if string(result) == "null" {
		if field.TagSettings["NOT NULL"] != "" {
			return "", nil
		}
		return nil, nil
	}
	return Encrypt(string(result))
}

// dbString normalizes the value the driver hands back to a string. GORM
// passes the raw DB value; the sqlite driver returns string for TEXT columns,
// but []byte and nil are defensively handled too.
func dbString(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []byte:
		return string(t)
	default:
		return ""
	}
}
