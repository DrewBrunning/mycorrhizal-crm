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
	"log"

	"mycorrhizal/atrest"
	"mycorrhizal/database"
)

func main() {
	dbPath := flag.String("db", "mycorrhizal.db", "path to the SQLite database file")
	newKey := flag.String("new", "", "new master key (base64, 32 bytes) to rewrap the DEK under")
	oldKey := flag.String("old", "", "old master key (base64, 32 bytes); defaults to the same resolution the server uses")
	flag.Parse()

	if *newKey == "" {
		log.Fatal("usage: rotate-at-rest-key -new <base64-key> [-old <base64-key>] [-db <path>]")
	}

	db, err := database.InitDB(*dbPath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}

	var old []byte
	if *oldKey != "" {
		kek, err := atrest.DecodeMasterKey(*oldKey)
		if err != nil {
			log.Fatalf("failed to decode -old key: %v", err)
		}
		old = kek
	} else {
		kek, err := atrest.EncryptionKey()
		if err != nil {
			log.Fatalf("failed to resolve current master key: %v", err)
		}
		if kek == nil {
			log.Fatal("no current master key resolved (DATA_ENCRYPTION_KEY/_FILE/JWT_SECRET_KEY all unset); pass -old explicitly")
		}
		old = kek
	}

	newKek, err := atrest.DecodeMasterKey(*newKey)
	if err != nil {
		log.Fatalf("failed to decode -new key: %v", err)
	}

	if err := atrest.RotateMasterKey(db, old, newKek); err != nil {
		log.Fatalf("rotation failed: %v", err)
	}

	log.Println("Master key rotated: DEK rewrapped under the new key. Set DATA_ENCRYPTION_KEY to the new key before restarting the server.")
}
