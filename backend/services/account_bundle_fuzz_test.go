package services

import (
	"encoding/json"
	"testing"

	"mycorrhizal/models"
)

// accountBundleFuzzSeeds seed the account-bundle decode+validate target. There
// is no checked-in bundle *file* fixture — the bundle is a per-user export and
// the checked-in examples live as struct literals in tests — so the valid seed
// is built here from models.AccountBundle exactly as the export/import tests do,
// and the malformed shapes come from the upload guard tests
// (account_bundle_coverage_test.go): non-JSON, correct format with a wrong
// version, and an empty object.
var accountBundleFuzzSeeds = [][]byte{
	func() []byte {
		b, err := json.Marshal(models.AccountBundle{
			Format:  models.AccountBundleFormat,
			Version: models.AccountBundleVersion,
			Plan: models.AccountBundlePlan{
				Contacts: []models.AccountBundleContact{{UID: "uid-fuzz-seed"}},
			},
		})
		if err != nil { // # pragma: no cover — marshalling a plain struct cannot fail
			panic(err)
		}
		return b
	}(),
	[]byte(`{"format":"mycorrhizal-account","version":1,"plan":{"contacts":[]}}`),
	[]byte(`{"format":"mycorrhizal-account","version":999}`),
	[]byte(`{"format":"not-a-bundle","version":1}`),
	[]byte(`{}`),
	[]byte(`not json`),
	{},
}

// FuzzAccountBundleUpload covers issue #1625's account-bundle target: the
// decode-and-validate step of (*MycorrhizalImportManager).Upload is the
// untrusted boundary for the `mycorrhizal` import source — the uploaded bundle
// is parsed as JSON and its format/version checked before anything is mapped
// (ADR 0028 Decision 3).
//
// Upload takes a *multipart.FileHeader, and no []byte decode+validate seam
// exists, so each input is wrapped in a real in-memory multipart body the same
// way the router's FormFile would present it (reusing the package's existing
// bundleMultipartHeader helper). Upload performs the decode and validation
// purely in memory — it does not touch the database (session creation is just a
// map insert) — so no dbtest.DB is needed here; the issue's dbtest-New-once
// guidance applies only when a db-backed helper is on the path, which it is not.
//
// The harness only needs to not panic; go test -fuzz catches panics
// automatically. The extra invariant is Upload's contract: a rejected input
// returns an error and no response, an accepted input returns a usable session
// id and no error, and the two are never mixed.
func FuzzAccountBundleUpload(f *testing.F) {
	for _, seed := range accountBundleFuzzSeeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > MaxMycorrhizalBundleSize {
			return
		}
		mgr := NewMycorrhizalImportManager()
		resp, appErr := mgr.Upload(1, bundleMultipartHeader(t, data))
		if appErr != nil {
			if resp != nil {
				t.Fatalf("Upload returned a response alongside error %v: %+v", appErr, resp)
			}
			return
		}
		if resp == nil {
			t.Fatal("Upload returned a nil response with a nil error")
		}
		if resp.SessionID == "" {
			t.Fatal("Upload accepted a bundle but returned an empty session id")
		}
	})
}
