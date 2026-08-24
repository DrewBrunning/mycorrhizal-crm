# Security checklist: OWASP MASVS-L1 (Android client)

The Android companion to `asvs-l2.md` (which covers backend + frontend + deployment). This file is
the living answer to "is the mobile app secure?" — a per-control map of the OWASP MASVS Level 1
requirements to the Kotlin source and CI that satisfy them. If a control's status changes, the PR
that changes it updates this file. A security-sensitive Android PR should touch the relevant row
here; if it cannot point at a row, it does not know what it is changing.

| | |
|---|---|
| **Standards pinned** | OWASP MASVS 1.5.0, chapters V2–V7 (the `MSTG-*` control IDs). MASVS 2.0 renamed these one-to-one to `MASVS-*` (V2–V7); the L1 rows below are the same either way. |
| **Level** | **MASVS-L1** (rows marked `x` in the L1 column). L2- and R-only rows are listed per chapter as out of scope. |
| **Last full pass** | 2026-08-24 (audit of `android/` + the Android CI jobs) |
| **Scope** | The Android client (`android/`): Kotlin + Jetpack Compose, Hilt, OkHttp, Room, DataStore, `androidx.security:crypto`. The server it talks to is covered by `asvs-l2.md`; MSTG-AUTH rows therefore cite the *client-side* contract and point at the backend for enforcement. |

## Status legend

| Status | Meaning |
|---|---|
| **satisfied** | Control is met; the row cites `file:line` or a test name. |
| **partial** | Control is met in part, or met with a documented deviation (each `partial` row names the gap). |
| **not-applicable** | Control does not apply (no WebView, no native code, no third-party SDK, …) — one-line reason given. |
| **out-of-scope** | L2/R-only control (not in the L1 target); listed for grep-ability. |

"Answered in the doc" means: take any `MSTG-xxxx-y` from MASVS 1.5.0, grep for it below, and get
a status + citation. No row is left `satisfied` without a citation.

## Documented positions

The `android/.mobsf` config already records a set of deliberate scanner findings as ignore-list
entries with one-line rationales. These are positions, not code changes — they are promoted here
with their full rationale so the audit is self-contained.

### P1 — No certificate pinning (MSTG-NETWORK-4 is L2 anyway)

Certificate pinning / transparency (`android_certificate_pinning`, `android_certificate_transparency`,
`android_ssl_pinning`) is deliberately not done. MSTG-NETWORK-4 is an L2 control, so it is out of
scope for L1 — but the ignore-list is explicit rather than silent. Rationale: the app talks to a
**self-hosted** server behind an operator-managed reverse proxy (`docs/deployment.md:17`), the same
trust model the backend itself uses (standard TLS, no pinning — see `asvs-l2.md` V9). Pinning a
self-hosted origin would break every cert rotation without a realistic MITM threat to answer to.
Self-signed server certs are handled via the **KeyChain import flow** (`installCustomCertificate`),
never by disabling TLS verification — see the production `network_security_config.xml` comment.

### P2 — Debug-only cleartext to the emulator loopback

The debug source set (`app/src/debug/res/xml/network_security_config.xml:19-28`) permits cleartext
HTTP to `10.0.2.2` / `127.0.0.1` / `localhost` so `adb`-driven local-backend testing works. This
file **never ships in a release build** — the production config forbids cleartext unconditionally
(`app/src/main/res/xml/network_security_config.xml:18`). Ignored at INFO (`android_manifest_domain_config_cleartext`)
so it stays visible in the source comment rather than adding CI noise.

### P3 — No root/SafetyNet/screenshot/tapjacking/anti-task-hijacking hardening

`android_root_detection`, `android_safetynet`/`android_safetynet_api`, `android_prevent_screenshot`,
`android_detect_tapjacking`, `android_tapjacking`, and the task-hijacking rules
(`android_task_hijacking1/2`) are ignored. Rationale: these are L2/R (resilience) or
defense-in-depth controls that do not fit a single-user personal CRM, and several (SafetyNet) are
deprecated platform services. The one task-hijacking adjacent risk that *is* real is addressed by
design: the exported `MainActivity` uses `launchMode="singleTask"` specifically so the OIDC deep
link is delivered via `onNewIntent` rather than stacking a second instance
(`app/src/main/AndroidManifest.xml:57`, comment at `:66-68`).

### P4 — The Room cache is not encrypted at rest

The Room database (`mycorrhizal-cache.db`, `core/data/.../DataModule.kt:126-144`) is **not**
SQLCipher-encrypted. This is the single substantive `partial` in STORAGE-1. Rationale: it is a
read-through cache mirroring server data for offline viewing (`AppDatabase.kt` doc comment), not the
source of truth — the database can be rebuilt from the server at any time
(`fallbackToDestructiveMigration`, `DataModule.kt:143`). The one exception is the
`pending_interactions` outbox (queued call/SMS tracking), which is genuinely sensitive metadata and
not rebuildable; it is documented below (STORAGE-1) as the reason the row is `partial` rather than
`satisfied`. Mitigating context: the file is inside the app sandbox, backups are disabled
(STORAGE-8), and the sensitive contacts themselves live server-side. Adopting SQLCipher or moving
the outbox to an encrypted store is the pre-1.0 follow-up.

### P5 — `.mobsf` false positives (not positions, recorded for the record)

`android_kotlin_hardcoded` fires only on the `const val SECRET = "secret"` sensitivity enum value
(the `normal|private|secret` taxonomy shared with the backend) — a domain value, not a credential.
`android_kotlin_sql_raw_query` fires only on the static Room-migration DDL strings
(`core/data/.../Migrations.kt`, `Migration13To14Test.kt`) — compile-time SQL with no injection
surface.

---

## V2 — Data Storage and Privacy (MSTG-STORAGE)

L2-only, out of scope: STORAGE-8, STORAGE-9, STORAGE-10, STORAGE-11, STORAGE-13, STORAGE-14, STORAGE-15.
(STORAGE-8 is met anyway — backups are disabled: `allowBackup="false"` and `data_extraction_rules` exclude
every domain, `app/src/main/AndroidManifest.xml:38-39`, `res/xml/data_extraction_rules.xml`.)

| ID | Requirement (abbrev.) | Status | Evidence |
|---|---|---|---|
| STORAGE-1 | Sensitive data in system credential storage | partial | The JWT is stored in `EncryptedSharedPreferences` with a Keystore `MasterKey` (AES256_GCM) — `core/data/.../EncryptedTokenStorage.kt:14-25`, DI binding `SessionStorageModule.kt:20-21`. **Gap:** the Room cache holds cached contact PII + the `pending_interactions` call/SMS outbox unencrypted (P4). |
| STORAGE-2 | No sensitive data outside app container / credential storage | satisfied | All persistence is inside the sandbox (EncryptedSharedPreferences, DataStore, Room). No external/shared storage for app data; the only `content://` surface is the FileProvider cache for user-initiated vCard sharing (`app/src/main/AndroidManifest.xml:78-86`). |
| STORAGE-3 | No sensitive data written to logs | satisfied | The JWT is never logged (`EncryptedTokenStorage.kt:10` comment); OkHttp debug logging is `Level.BASIC` (request line only, no headers/body) and debug-only (`core/network/.../NetworkFactory.kt:35-41`). |
| STORAGE-4 | No sharing with third parties unless necessary | satisfied | No telemetry/analytics/third-party SDKs; Firebase is applied only when a real `google-services.json` exists and is otherwise inert (`app/build.gradle.kts:13-15`, `AndroidManifest.xml:98-105`). vCard export is a user-initiated system share sheet (`AndroidManifest.xml:76-86`). |
| STORAGE-5 | Keyboard cache disabled on sensitive inputs | partial | Register / forgot-password / settings / users secret fields use `KeyboardType.Password` (the Android signal that disables IME personalized learning) alongside `PasswordVisualTransformation` — `feature/auth/.../RegisterScreen.kt:151-152`, `ForgotPasswordScreen.kt:147-148,156-157`, `feature/settings/.../SettingsScreen.kt:263-264`. **Gap:** the login screen's password field (`feature/auth/.../LoginScreen.kt:193-199`) masks via `PasswordVisualTransformation` + autofill `ContentType.Password` but keeps default keyboard options — no `KeyboardType.Password` on the one field where a password is typed most often. |
| STORAGE-6 | No sensitive data exposed via IPC | satisfied | The only exported components are system-driven: `MainActivity` (launcher + OIDC deep link) and the `PHONE_STATE`/`SMS_RECEIVED` receivers (the latter guarded by `android.permission.BROADCAST_SMS`); everything else is `exported="false"` (`AndroidManifest.xml:78-129`). The FileProvider is `exported="false"` with per-URI grants (`:80-81`). |
| STORAGE-7 | No sensitive data exposed via UI | satisfied | Passwords are masked via `PasswordVisualTransformation` on all secret fields (see STORAGE-5 citations); no secret is rendered readably. |
| STORAGE-12 | App educates user on PII handling | satisfied | Self-hosted personal CRM: the privacy stance (data stays on the operator's server) is documented in `docs/deployment.md` and `CLAUDE.md`; no third-party collection exists to disclose. |

## V3 — Cryptography (MSTG-CRYPTO)

No L2-only rows in this chapter (CRYPTO-1…6 are all L1).

| ID | Requirement (abbrev.) | Status | Evidence |
|---|---|---|---|
| CRYPTO-1 | No hardcoded keys as sole encryption | satisfied | No symmetric keys in source; the Keystore `MasterKey` (AES256_GCM) is hardware-backed (`EncryptedTokenStorage.kt:15-17`). mobsfscan's `android_kotlin_hardcoded` fires only on the `SECRET` enum value (P5). |
| CRYPTO-2 | Proven cryptographic implementations | satisfied | `androidx.security:crypto` (`EncryptedSharedPreferences`, `MasterKey`) — the platform-recommended library, no hand-rolled crypto (`EncryptedTokenStorage.kt:5-6`). |
| CRYPTO-3 | Appropriate primitives, best-practice parameters | satisfied | Master key `AES256_GCM`; pref keys `AES256_SIV`; pref values `AES256_GCM` (`EncryptedTokenStorage.kt:16,22-23`) — AES-256 with authenticated (GCM) and key-wrapping (SIV) modes, the library's recommended pairing. |
| CRYPTO-4 | No deprecated algorithms | satisfied | No MD5/SHA1/DES/ECB/Blowfish anywhere in `android/`; only AES-256-GCM/SIV via the platform library. |
| CRYPTO-5 | No key reuse across purposes | satisfied | A single purpose (session token at rest) uses the single Keystore master key; all other persisted state (DataStore prefs, Room cache) is non-secret. No key is reused across different secrets. |
| CRYPTO-6 | Secure random number generator | satisfied | No custom RNG; the Keystore master key and `androidx.security.crypto` generate keys/nonces internally with the platform CSPRNG. |

## V4 — Authentication and Session Management (MSTG-AUTH)

The app is a thin client: authentication and authorization are enforced at the remote endpoint
(covered in `asvs-l2.md` V2/V3/V4). Client-side rows cite the client contract.

L2-only, out of scope: AUTH-8, AUTH-9, AUTH-10, AUTH-11.

| ID | Requirement (abbrev.) | Status | Evidence |
|---|---|---|---|
| AUTH-1 | Authentication performed at the remote endpoint | satisfied | Username/password login (`feature/auth/.../LoginScreen.kt`, `LoginViewModel`) and OIDC (`feature/auth`) both submit to the backend; nothing is authenticated client-side only. |
| AUTH-2 | Stateful sessions use random server ids | not-applicable | Sessions are stateless JWTs, not stateful server ids — the `not-applicable` is the clean read; see AUTH-3. |
| AUTH-3 | Stateless tokens signed with a secure algorithm | satisfied | The backend issues HS256 JWTs (`asvs-l2.md` 3.2.4); the client treats the JWT as an opaque bearer credential and validates it against `/users/me` before flipping to logged-in (`MainActivity.kt:193-212`). |
| AUTH-4 | Remote endpoint terminates session on logout | satisfied | Backend logout + `token_version` revocation (`asvs-l2.md` 3.3.1); client `clearSession()` drops the token (`DefaultSessionManager`), leaving nothing to replay. |
| AUTH-5 | Password policy enforced at remote endpoint | satisfied | Entropy-based policy + strength meter (`asvs-l2.md` 2.1.1/2.1.8); client mirrors it in the register form. |
| AUTH-6 | Anti-automation against credential submission | satisfied | Per-account lockout + per-IP limiter at the endpoint (`asvs-l2.md` 2.2.1); client surfaces the 429 as a generic failure. |
| AUTH-7 | Session invalidation after inactivity, token expiry | satisfied | JWT 96 h absolute expiry + `token_version` revocation (`asvs-l2.md` 3.3.1/3.3.2); client handles 401 by clearing the session. |
| AUTH-12 | Authorization model enforced at remote endpoint | satisfied | Ownership scoping is entirely server-side (`asvs-l2.md` V4); the client only renders what the API returns — no client-side authz to bypass. |

## V5 — Network Communication (MSTG-NETWORK)

L2-only, out of scope: NETWORK-4, NETWORK-5, NETWORK-6.

| ID | Requirement (abbrev.) | Status | Evidence |
|---|---|---|---|
| NETWORK-1 | TLS consistently, no cleartext | satisfied | Production network security config forbids cleartext (`cleartextTrafficPermitted="false"`) and trusts only system CAs — `app/src/main/res/xml/network_security_config.xml:17-23`, wired via `AndroidManifest.xml:43`. Pinned by test (see `NetworkSecurityConfigTest`). Debug-only loopback cleartext never ships (P2). |
| NETWORK-2 | TLS settings align with best practice | satisfied | The app lets the platform's TLS stack negotiate (no custom socket factory, no `sslSocketFactory` overrides anywhere); current OkHttp enforces TLS 1.2+. |
| NETWORK-3 | Verify X.509, only trusted CAs | satisfied | Trust anchors are `system` only (`network_security_config.xml:20`); user-installed CAs are trusted **only** in the debug variant (`debug/res/xml/network_security_config.xml:30-35`). No custom trust manager or `X509TrustManager` override. |

## V6 — Platform Interaction (MSTG-PLATFORM)

L2-only, out of scope: PLATFORM-9, PLATFORM-10, PLATFORM-11.

| ID | Requirement (abbrev.) | Status | Evidence |
|---|---|---|---|
| PLATFORM-1 | Minimum necessary permissions | satisfied | Permissions are runtime, opt-in, and documented inline with rationale: `READ_CONTACTS` (link-action resolver, requested inline — `AndroidManifest.xml:7-12`), call-log/SMS/phone tracking (`:14-17`), notifications, overlay, foreground service, boot (`:18-26`). `uses-feature ... required="false"` so tablets/ChromeOS install without telephony hardware (`:30-31`). |
| PLATFORM-2 | Validate + sanitize all external input | satisfied | The OIDC deep link is strictly validated — scheme, host, **and** path — so an exported `MainActivity` cannot be driven by a foreign explicit-component VIEW intent (`MainActivity.kt:245-259`); pinned by `OidcReturnParsingTest` (`app/src/test/.../OidcReturnParsingTest.kt:53-58`). Notification deep links are parsed into known routes only (`MainActivity.kt:226-231`). |
| PLATFORM-3 | No sensitive functionality via custom URL schemes unless protected | satisfied | The only custom scheme is `mycorrhizal://`, and its one token-bearing route (`/oidc/callback`) carries the OIDC JWT — the scheme is unique to the app, the callback path is fixed, and an invalid/stale token is rejected before the session flips (`MainActivity.kt:193-212`). |
| PLATFORM-4 | No sensitive functionality via IPC unless protected | satisfied | No `ContentProvider` exposes app data (the FileProvider is `exported="false"` + per-URI grants); the exported receivers are system broadcasts only, and the SMS receiver requires `BROADCAST_SMS` (`AndroidManifest.xml:110-129`). |
| PLATFORM-5 | JavaScript disabled in WebViews | not-applicable | No WebView anywhere in the app (grep for `WebView`/`CustomTabs` returns nothing); OIDC login opens the system browser. |
| PLATFORM-6 | WebView protocol handlers minimized | not-applicable | No WebView (see PLATFORM-5). |
| PLATFORM-7 | Native bridges only render in-package JS | not-applicable | No WebView / `addJavascriptInterface` anywhere. |
| PLATFORM-8 | Safe deserialization | satisfied | Moshi (`NetworkFactory.kt:21,27-42`) with codegen (`@JsonClass`) — reflection-free, no `ObjectInputStream`/`Parcelable`-of-untrusted-data, no pickle/Java serialization. |

## V7 — Code Quality and Build Settings (MSTG-CODE)

No L2-only rows in this chapter.

| ID | Requirement (abbrev.) | Status | Evidence |
|---|---|---|---|
| CODE-1 | Signed with valid cert, private key protected | satisfied | Release signing via env/properties only, never committed; a partial set fails fast rather than producing a null-password APK (`app/build.gradle.kts:17-32`). |
| CODE-2 | Release-mode build, non-debuggable | satisfied | `isMinifyEnabled = true`, `isShrinkResources = true`, ProGuard rules for R8 (`app/build.gradle.kts:55-65`, `app/proguard-rules.pro`); release builds are non-debuggable by AGP default; `./gradlew assembleRelease` is gated in `.github/workflows/android-apk-build.yml` (signing secrets required, fail-fast) and `docker-publish.yml:200`. |
| CODE-3 | Debug symbols removed from native binaries | not-applicable | No native code (pure Kotlin/JVM); nothing to strip. |
| CODE-4 | No debug/backdoor code, no verbose logging | satisfied | HTTP logging is debug-build-only and `Level.BASIC` (`NetworkFactory.kt:35-41`); no hidden settings/backdoors; `detekt` in CI gates dead code and leftover debug paths (`android-tests.yml` → `./gradlew detekt`). |
| CODE-5 | Third-party components identified + vulnerability-checked | satisfied | CodeQL `java-kotlin` (`codeql.yml:102-122`), mobsfscan (`sast.yml`), Android Lint, `detekt`, and Dependabot's `gradle` ecosystem (`/android`, `.github/dependabot.yml`) + GitHub Dependency Review (`dependency-review.yml`). `filters.yaml` maps `android/**` to the Android + SAST + CodeQL jobs. |
| CODE-6 | Exceptions caught and handled | satisfied | Result/`fold` error handling across repositories and the OIDC flow (`MainActivity.kt:197-212`); `detekt`'s over-broad-exception rule in CI. |
| CODE-7 | Error handling denies access by default | satisfied | 401/404s clear the session rather than leaking data; failed profile fetch flips to logged-out (`MainActivity.kt:208-211`); no client-side fallback that reveals data on error. |
| CODE-8 | Unmanaged-code memory handled securely | not-applicable | No unmanaged/native code. |
| CODE-9 | Free toolchain security features enabled | satisfied | R8 byte-code minification + resource shrink (`app/build.gradle.kts:57-58`), ProGuard rules for reflection-using libs (`app/proguard-rules.pro`), and the platform's default hardening (NX, ASLR) apply to the managed app. |
