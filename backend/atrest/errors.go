package atrest

import "errors"

// Sentinel errors, one per distinct failure class (issue #1594). Callers and
// tests match them with errors.Is; the human-readable messages operators see
// are unchanged. No error ever carries key material.
var (
	// ErrNotInitialized: an encrypted value was read while the at-rest layer
	// is not armed (fail closed rather than hand back ciphertext).
	ErrNotInitialized = errors.New("atrest: encrypted value read while at-rest encryption is not initialized")
	// ErrMalformed: a stored value or ciphertext is structurally invalid
	// (bad prefix/key id/base64, or shorter than a GCM nonce).
	ErrMalformed = errors.New("atrest: malformed encrypted value")
	// ErrAuthFailed: the AEAD open failed — wrong key or tampered data.
	ErrAuthFailed = errors.New("atrest: authentication failed")
	// ErrDBRequired: an operation needs a database handle and got nil.
	ErrDBRequired = errors.New("atrest: db handle required")
	// ErrInvalidKey: master-key material is missing or not a valid
	// base64-encoded 32-byte key.
	ErrInvalidKey = errors.New("atrest: invalid master key")
	// ErrNoWrappedDEK: rotation found no wrapped data-encryption key.
	ErrNoWrappedDEK = errors.New("atrest: rotate: no wrapped DEK found (has the server ever booted with a key?)")
	// ErrMalformedSpec: an EncryptedColumns entry is not "table.column".
	ErrMalformedSpec = errors.New("atrest: malformed encrypted-column spec")
)

// classedError keeps its own operator-facing message while unwrapping to a
// sentinel class and (optionally) the underlying cause.
type classedError struct {
	class error
	msg   string
	cause error
}

func (e *classedError) Error() string { return e.msg }

func (e *classedError) Unwrap() []error {
	if e.cause == nil {
		return []error{e.class}
	}
	return []error{e.class, e.cause}
}

func classify(class error, msg string, cause error) error {
	return &classedError{class: class, msg: msg, cause: cause}
}
