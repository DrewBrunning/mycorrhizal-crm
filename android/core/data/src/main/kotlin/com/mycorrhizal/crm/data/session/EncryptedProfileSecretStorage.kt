package com.mycorrhizal.crm.data.session

/**
 * Production [ProfileSecretStorage]: moves a pre-profiles install's device
 * grant (issue #722) and pending OIDC request (issue #965) from their legacy
 * single-profile slots onto the migrated profile's `:<profileId>` slots, and
 * drops a profile's secrets when it is removed (ADR 0028 Decision 1).
 */
class EncryptedProfileSecretStorage(
    private val deviceGrantStorage: DeviceGrantTokenStorage,
    private val oidcPendingStore: OidcPendingRequestStore,
) : ProfileSecretStorage {

    override suspend fun migrateLegacy(profileId: String) {
        deviceGrantStorage.loadLegacy()?.let { grant ->
            deviceGrantStorage.save(profileId, grant.token, grant.id ?: 0L)
        }
        deviceGrantStorage.clearLegacy()
        oidcPendingStore.loadLegacy()?.let { pending ->
            oidcPendingStore.save(profileId, pending.state, pending.codeVerifier)
        }
        oidcPendingStore.clearLegacy()
    }

    override suspend fun clear(profileId: String) {
        deviceGrantStorage.clear(profileId)
        oidcPendingStore.clear(profileId)
    }
}
