package com.mycorrhizal.crm.data.session

/** In-memory [TokenStorage] for unit tests. */
class FakeTokenStorage : TokenStorage {
    var stored: String? = null
    override suspend fun save(token: String) { stored = token }
    override suspend fun load(): String? = stored
    override suspend fun clear() { stored = null }
}

/** In-memory [SessionPrefsStorage] for unit tests. */
class FakeSessionPrefsStorage : SessionPrefsStorage {
    var stored: String? = null
    override suspend fun save(serverUrl: String?) { stored = serverUrl }
    override suspend fun loadServerUrl(): String? = stored
    override suspend fun clear() { stored = null }
}
