package com.mycorrhizal.crm.domain.compat

import com.mycorrhizal.crm.model.network.ServerHealth

/** Projects the compatibility fields of a `/health` body onto the capability model. */
fun ServerHealth.toCapabilitiesInfo(): ServerCapabilitiesInfo =
    ServerCapabilitiesInfo(deployment = deployment, capabilities = capabilities?.toSet())

/**
 * The last capability set resolved for the active profile, shared between the
 * root (which fetches `/health` once per session) and consumers that act on a
 * capability outside the Compose tree — notably `DeviceRegistrationManager`,
 * which must not register for FCM push against a deployment that does not offer
 * it. Its default ([ServerCapabilitiesInfo.Unknown]) fails open.
 */
interface ServerCapabilitiesStore {
    fun current(): ServerCapabilitiesInfo

    /** Records the set resolved for the current session. */
    fun record(info: ServerCapabilitiesInfo)
}

/**
 * ADR 0028 Decision 2 / issue #1263: the capability set a connected server
 * declares on `GET /health`, distinct from the version-based [ServerFeature]
 * gate. `capabilities` is the backend's `config.Capabilities()` list; a
 * deployment simply omits the surfaces it does not register, so the client
 * gates on **token presence**, never on "is this server local".
 *
 * ## Fail-open
 *
 * A null [capabilities] (an older server that predates the field, or an
 * unreachable/garbled `/health`) means every capability is treated as present —
 * this preserves the client's existing fail-open posture and never hides
 * functionality because the server could not be interrogated.
 */
data class ServerCapabilitiesInfo(
    /** "server" or "embedded"; null when an older server omits it. */
    val deployment: String? = null,
    /** The declared surface tokens, or null when the field is absent (fail open). */
    val capabilities: Set<String>? = null,
) {
    /**
     * True when [token] is available. Fail-open: an absent list is "everything".
     * A present-but-empty list is "nothing", which is a real (if degenerate)
     * server declaration and must be honoured.
     */
    fun supports(token: String): Boolean = capabilities?.contains(token) ?: true

    /** True only when the server explicitly declares the embedded deployment. */
    val isEmbedded: Boolean get() = deployment == DEPLOYMENT_EMBEDDED

    companion object {
        const val DEPLOYMENT_SERVER = "server"
        const val DEPLOYMENT_EMBEDDED = "embedded"

        /** The absent-field value: deployment and capabilities both unknown. */
        val Unknown = ServerCapabilitiesInfo()
    }
}

/**
 * The capability tokens this UI gates on. Hardcoded mirrors of the backend's
 * `config` constants (backend/config/config.go) — there is no dynamic token
 * endpoint by design (frontend trap #4), so **this list must be kept in sync by
 * hand** when a token is added or renamed server-side.
 */
object ServerCapability {
    // Surfaces the embedded deployment omits.
    const val REGISTRATION = "registration"
    const val LOGIN = "login"
    const val PASSWORD_RESET = "password_reset"
    const val OIDC = "oidc"
    const val TWO_FACTOR = "two_factor"
    const val EMAIL = "email"
    const val API_TOKENS = "api_tokens"
    const val CONTACT_SHARES = "contact_shares"
    const val WEBHOOKS = "webhooks"
    const val CARDDAV = "carddav"
    const val CALDAV = "caldav"
    const val DEVICE_GRANTS = "device_grants"
    const val PUSH = "push"

    // Surfaces that exist on every deployment (kept for completeness / future
    // gates; the version gate already covers them).
    const val CONTACTS = "contacts"
    const val DASHBOARD = "dashboard"
    const val NOTES = "notes"
    const val ACTIVITIES = "activities"
    const val REMINDERS = "reminders"
    const val LIFE_EVENTS = "life_events"
    const val GRAPH = "graph"
    const val SEARCH = "search"
    const val IMPORT = "import"
    const val EXPORT = "export"
    const val CALENDAR = "calendar"
    const val NOTIFICATIONS = "notifications"
}
