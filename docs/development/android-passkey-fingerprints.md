# Android passkey signing fingerprints — runbook (issue #1334)

Native Android passkeys work only for signing certificates listed in the server-generated
`/.well-known/assetlinks.json` (ADR [0034](../adrs/0034-android-passkeys.md)).
Each release channel is signed by a different key (ADR [0022](../adrs/0022-distribution-variants.md)),
so each needs its own entry. The reviewed list is `channels` in
[`backend/androidpasskey/androidpasskey.go`](../../backend/androidpasskey/androidpasskey.go); the
current values and their provenance are in
[`release-verification.md`](../security/release-verification.md#signing-certificate-fingerprints-native-passkeys).

**Never guess or generate a value.** Every entry comes from real signing material, with its source
and the date recorded.

## Adding a channel's fingerprint

| Channel | Which certificate | Where to read it |
|---|---|---|
| `obtainium` | the project release keystore | `apksigner verify --print-certs` on a published `app-obtainium-release.apk`, cross-checked with `keytool -list -v -keystore <jks> -alias <alias>` |
| `play` | Google's **App signing key certificate**, *not* the upload key | Play Console → Test and release → App integrity → App signing (exists only after Play App Signing enrollment and a first AAB upload) |
| `foss` | F-Droid's per-app key (the recipe sets neither `Binaries` nor `AllowedAPKSigningKeys`) | `apksigner verify --print-certs` on the APK downloaded from f-droid.org, **and** the signer on the package page / repo index; the two must agree |

1. Fill the channel's entry in `channels` (colon-separated uppercase; the parser accepts either form),
   with a comment naming the source and date.
2. Add a row to the table in `release-verification.md`.
3. Update the fixtures that pin the registry (`backend/androidpasskey/androidpasskey_test.go`,
   `backend/controllers/webauthn_androidpasskeys_test.go`, `backend/embedded/passkeys_native_test.go`).
4. Ship a **server** release. The file is served by the server, so no app release is needed for a
   fingerprint to start working. Say so in the release notes.
5. Run the verification below.

## Verification (per shipped channel, on a real device)

Use a public-HTTPS instance with `WEBAUTHN_ANDROID_ENABLED=true` and `FRONTEND_URL` set to its public URL.

1. `curl -sI https://<host>/.well-known/assetlinks.json` → `200`, `application/json`, no redirect.
2. Ask Google's Digital Asset Links API whether it sees the statement; the channel's package name and
   fingerprint must be listed:
   `https://digitalassetlinks.googleapis.com/v1/statements:list?source.web.site=https://<host>&relation=delegate_permission/common.get_login_creds`
3. Install that channel's APK, enroll a passkey, sign out, sign back in with it.

A channel that fails step 3 while steps 1–2 pass has a fingerprint mismatch, not a server problem.
Record the result per channel on issue #1334.

## Rotation and new channels

- **New channel or new signing key** (a Play app-signing key upgrade, an F-Droid key rotation, a fourth
  store): add an entry, ship a server release, re-run the verification. The registry supports several
  entries per channel.
- **Never remove the old entry** until APKs signed with the old key are no longer in the wild. Removing
  it early silently breaks passkey login for those users.
- **Who watches for it:** Play Console shows a pending key upgrade; F-Droid announces rotations on its
  side. Neither is automated yet, so check both before each Android release; a rotation should not be discovered from a user report.
- **Forks and self-built APKs** that sign with their own key do not edit the registry; they set
  `WEBAUTHN_ANDROID_CERT_SHA256` (see [`deployment.md`](../deployment.md)).
