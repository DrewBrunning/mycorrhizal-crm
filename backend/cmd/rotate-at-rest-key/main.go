// Command rotate-at-rest-key rewraps the deployment's data-encryption key
// (DEK) under a new master key (issue #380, ASVS V6.4.1). The payload bytes
// are never touched — rotation only unwraps the DEK with the old master key
// and rewraps it with the new one, updating the single data_encryption_keys
// row. After running this, set DATA_ENCRYPTION_KEY (or DATA_ENCRYPTION_KEY_FILE)
// to the new key so the server boots with it.
//
// "Lost key = lost data, by design": the new master key must be backed up the
// same way the old one was. If both are lost, the wrapped DEK cannot be
// unwrapped and every encrypted column becomes undecryptable.
//
// Usage:
//
//	go run cmd/rotate-at-rest-key/main.go -new <new-base64-key> [-db <path>]
//
// The old key is resolved the same way the server resolves it: DATA_ENCRYPTION_KEY,
// then DATA_ENCRYPTION_KEY_FILE, then the HKDF derivation from JWT_SECRET_KEY.
// Pass -old to override explicitly (e.g. when rotating from a previously
// explicit key to a new one without touching the running env).
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"mycorrhizal/atrest"
	"mycorrhizal/database"
)

func main() {
	os.Exit(run(os.Args[1:], os.Getenv, os.Stdout, os.Stderr)) // # pragma: no cover — os.Exit terminates the process; tests exercise run() directly
}

// run is split out of main so the exit paths are testable. Exit codes: 0 on
// success, 2 on a usage/parse error, 1 on any runtime failure.
func run(args []string, env func(string) string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("rotate-at-rest-key", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dbPath := fs.String("db", "mycorrhizal.db", "path to the SQLite database file")
	newKey := fs.String("new", "", "new master key (base64, 32 bytes) to rewrap the DEK under")
	oldKey := fs.String("old", "", "old master key (base64, 32 bytes); defaults to the same resolution the server uses")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *newKey == "" {
		fmt.Fprintln(stderr, "usage: rotate-at-rest-key -new <base64-key> [-old <base64-key>] [-db <path>]")
		return 2
	}

	db, err := database.InitDB(*dbPath)
	if err != nil {
		fmt.Fprintf(stderr, "failed to open database: %v\n", err)
		return 1
	}

	var old []byte
	if *oldKey != "" {
		kek, err := atrest.DecodeMasterKey(*oldKey)
		if err != nil {
			fmt.Fprintf(stderr, "failed to decode -old key: %v\n", err)
			return 1
		}
		old = kek
	} else {
		kek, err := atrest.ResolveMasterKey(env("DATA_ENCRYPTION_KEY"), env("DATA_ENCRYPTION_KEY_FILE"), env("JWT_SECRET_KEY"))
		if err != nil {
			fmt.Fprintf(stderr, "failed to resolve current master key: %v\n", err)
			return 1
		}
		if kek == nil {
			fmt.Fprintln(stderr, "no current master key resolved (DATA_ENCRYPTION_KEY/_FILE/JWT_SECRET_KEY all unset); pass -old explicitly")
			return 1
		}
		old = kek
	}

	newKek, err := atrest.DecodeMasterKey(*newKey)
	if err != nil {
		fmt.Fprintf(stderr, "failed to decode -new key: %v\n", err)
		return 1
	}

	if err := atrest.RotateMasterKey(db, old, newKek); err != nil {
		fmt.Fprintf(stderr, "rotation failed: %v\n", err)
		return 1
	}

	fmt.Fprintln(stdout, "Master key rotated: DEK rewrapped under the new key. Set DATA_ENCRYPTION_KEY to the new key before restarting the server.")
	return 0
}
