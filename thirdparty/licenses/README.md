# Bundled license texts

Canonical SPDX license texts used by:

- `backend/cmd/gennotices` when an npm package declares a license but ships no
  license file (`MIT`, `ISC`, `BSD-2-Clause`, `BSD-3-Clause`, `Apache-2.0`,
  `MPL-2.0`; fetched from the SPDX license-list-data project).
- `android/config/licenses/` as the offline source for AboutLibraries
  (`Apache-2.0`, `BSD-2-Clause`, the Android SDK License for the GMS
  dependencies in the `obtainium`/`play` flavors, plus SQLCipher's BSD-style
  license, which has no SPDX identifier in the AAR metadata).

These are reference texts, not a second mapping source: the generated
`THIRD_PARTY_NOTICES.md` is the record of which dependency uses which text.
