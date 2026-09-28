package com.mycorrhizal.crm.data.session

/** In-memory [TokenStorage] for unit tests, keyed by profile ID. */
class FakeTokenStorage : TokenStorage {
    val tokens = mutableMapOf<String, String>()

    /** The pre-profiles single `jwt` slot. */
    var legacy: String? = null

    /**
     * Convenience for single-profile tests. Reading returns the only saved
     * per-profile token, or the legacy slot. Writing sets the *legacy* slot
     * (the pre-profiles shape) — pair it with a legacy server URL in
     * [FakeSessionPrefsStorage] when the test calls `init()` so the manager
     * migrates it onto a profile.
     */
    var stored: String?
        get() = tokens.values.lastOrNull() ?: legacy
        set(value) {
            legacy = value
        }

    override suspend fun save(profileId: String, token: String) {
        tokens[profileId] = token
    }

    override suspend fun load(profileId: String): String? = tokens[profileId]

    override suspend fun clear(profileId: String) {
        tokens.remove(profileId)
    }

    override suspend fun loadLegacy(): String? = legacy

    override suspend fun clearLegacy() {
        legacy = null
    }
}

/** In-memory [SessionPrefsStorage] for unit tests. */
class FakeSessionPrefsStorage : SessionPrefsStorage {
    /** The pre-profiles server URL slot. */
    var serverUrl: String? = null

    var snapshot: ProfilesSnapshot = ProfilesSnapshot()

    override suspend fun save(serverUrl: String?) {
        this.serverUrl = serverUrl
    }

    override suspend fun loadServerUrl(): String? = serverUrl

    override suspend fun saveProfiles(snapshot: ProfilesSnapshot) {
        this.snapshot = snapshot
    }

    override suspend fun loadProfiles(): ProfilesSnapshot = snapshot

    override suspend fun clear() {
        serverUrl = null
        snapshot = ProfilesSnapshot()
    }
}
