package com.mycorrhizal.crm.data.passkey

import com.mycorrhizal.crm.data.session.SessionManager
import com.mycorrhizal.crm.domain.compat.ServerCapability
import com.mycorrhizal.crm.domain.compat.ServerCapabilitiesInfo
import com.mycorrhizal.crm.domain.compat.ServerCapabilitiesStore
import com.mycorrhizal.crm.domain.compat.toCapabilitiesInfo
import com.mycorrhizal.crm.domain.profile.ServerProfileKind
import com.mycorrhizal.crm.domain.repository.ServerCompatibilityRepository
import dagger.Binds
import dagger.Module
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import javax.inject.Inject

/**
 * Issue #1293 / ADR 0034 Decision 4 — the single gate for "can this build,
 * against this server, run a passkey ceremony right now". Consulted by the
 * login 2FA step, the Settings entry point and nothing else, so the rule lives
 * in exactly one place.
 */
fun interface PasskeyAvailability {
    suspend fun isAvailable(): Boolean
}

/**
 * True only when ALL of:
 *  - the device can run Credential Manager ([PasskeyCredentialClient.isSupported]);
 *  - the active profile is not the embedded Local profile (no domain, so no RP
 *    ID — ADR 0028); and
 *  - the server's `/health` explicitly declares `webauthn_android`, i.e. the
 *    operator enabled native Android passkeys on a valid public HTTPS instance.
 *
 * The capability set comes from [ServerCapabilitiesStore], which `MainViewModel`
 * refreshes on every server-URL change and login edge and resets to Unknown on
 * a switch, so a profile switch can never leave the previous server's token
 * behind. Unknown/absent capabilities are CLOSED here (unlike most gates): the
 * token is an operator opt-in, never assumed present.
 */
class DefaultPasskeyAvailability @Inject constructor(
    private val client: PasskeyCredentialClient,
    private val capabilitiesStore: ServerCapabilitiesStore,
    private val sessionManager: SessionManager,
    private val serverCompatibility: ServerCompatibilityRepository,
) : PasskeyAvailability {
    override suspend fun isAvailable(): Boolean {
        if (!client.isSupported()) return false
        if (sessionManager.activeProfile()?.kind is ServerProfileKind.Local) return false
        var info = capabilitiesStore.current()
        if (info == ServerCapabilitiesInfo.Unknown) {
            // The session-level /health check has not resolved (or failed): ask the
            // server directly rather than guess. Not recorded into the store — that
            // stays MainViewModel's single writer. An unreachable server stays closed.
            info = serverCompatibility.getServerHealth().getOrNull()?.toCapabilitiesInfo()
                ?: return false
        }
        if (info.isEmbedded) return false
        return info.declares(ServerCapability.WEBAUTHN_ANDROID)
    }
}

@Module
@InstallIn(SingletonComponent::class)
abstract class PasskeyModule {
    @Binds
    abstract fun bindPasskeyAvailability(impl: DefaultPasskeyAvailability): PasskeyAvailability

    @Binds
    abstract fun bindPasskeyCredentialClient(impl: CredentialManagerPasskeyClient): PasskeyCredentialClient
}
