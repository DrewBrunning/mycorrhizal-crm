package com.mycorrhizal.crm.data.compat

import com.mycorrhizal.crm.domain.compat.ServerCapabilitiesInfo
import com.mycorrhizal.crm.domain.compat.ServerCapabilitiesStore
import javax.inject.Inject
import javax.inject.Singleton

/**
 * In-memory holder for the capability set most recently resolved from `/health`.
 * Deliberately not persisted: capabilities belong to the live session, and a
 * stale set surviving a restart could hide a capability the server now offers
 * (or offer one it dropped). [ServerCapabilitiesInfo.Unknown] until the session's
 * first check resolves — the fail-open default.
 */
@Singleton
class DefaultServerCapabilitiesStore @Inject constructor() : ServerCapabilitiesStore {

    @Volatile
    private var info: ServerCapabilitiesInfo = ServerCapabilitiesInfo.Unknown

    override fun current(): ServerCapabilitiesInfo = info

    override fun record(info: ServerCapabilitiesInfo) {
        this.info = info
    }
}
