package com.mycorrhizal.crm.domain.repository

import com.mycorrhizal.crm.model.network.ApiToken
import com.mycorrhizal.crm.model.network.ApiTokenCreateResponse
import com.mycorrhizal.crm.model.network.ApiTokenInput
import com.mycorrhizal.crm.model.network.NotificationConfig
import com.mycorrhizal.crm.model.network.NotificationConfigInput
import com.mycorrhizal.crm.model.network.NotificationTestResult
import com.mycorrhizal.crm.model.network.RevokeAllApiTokensResponse
import com.mycorrhizal.crm.model.network.Webhook
import com.mycorrhizal.crm.model.network.WebhookCreateResponse
import com.mycorrhizal.crm.model.network.WebhookDelivery
import com.mycorrhizal.crm.model.network.WebhookInput
import kotlinx.coroutines.flow.Flow

/**
 * Webhook CRUD + test + delivery history (M25). Mirrors web's
 * `WebhooksSettings.tsx` data model exactly; the secret appears only on
 * create ([WebhookCreateResponse.secret]) and must never be logged or
 * persisted locally beyond that one-shot reveal dialog.
 */
interface WebhookRepository {
    suspend fun list(): Result<List<Webhook>>
    suspend fun create(input: WebhookInput): Result<WebhookCreateResponse>
    suspend fun update(id: Int, input: WebhookInput): Result<Webhook>
    suspend fun delete(id: Int): Result<Unit>
    suspend fun test(id: Int): Result<WebhookDelivery>
    suspend fun deliveries(id: Int): Result<List<WebhookDelivery>>
}

/**
 * API token lifecycle (issue #413's Android follow-up, #573): list, create,
 * revoke a single token, revoke every standing token at once, and rotate
 * (revoke + reissue under the same name/scope). Mirrors web's
 * `frontend/src/api/apiTokens.ts` exactly; the plaintext appears only on
 * [create] and [rotate] and must never be logged or persisted locally beyond
 * the one-shot reveal dialog.
 */
interface ApiTokenRepository {
    suspend fun list(): Result<List<ApiToken>>
    suspend fun create(input: ApiTokenInput): Result<ApiTokenCreateResponse>
    suspend fun revoke(id: Int): Result<Unit>
    suspend fun revokeAll(): Result<RevokeAllApiTokensResponse>
    suspend fun rotate(id: Int): Result<ApiTokenCreateResponse>
}

/**
 * Per-user ntfy/Gotify notification-channel config + per-channel test (M25).
 * [NotificationConfigInput.gotifyToken] is write-only — the stored token is
 * never returned, only [NotificationConfig.gotifyHasToken].
 */
interface NotificationSettingsRepository {
    suspend fun config(): Result<NotificationConfig>
    suspend fun save(input: NotificationConfigInput): Result<NotificationConfig>
    suspend fun test(channel: String): Result<NotificationTestResult>
}

/**
 * Device-side app settings that have no server endpoint (M25): the theme
 * choice and the locale override that drives string resolution. Theme values
 * are `system` | `light` | `dark`. The language override stores the user's
 * chosen language tag so the Activity's base context can resolve `values-XX`
 * resources even before any server round-trip (see [currentLanguageOverride]).
 */
interface AppSettingsRepository {
    companion object {
        const val THEME_SYSTEM = "system"
        const val THEME_LIGHT = "light"
        const val THEME_DARK = "dark"
    }

    /** Theme choice (`system` default) — recomposition-driven, so MainActivity can react live. */
    fun themePreference(): Flow<String>
    suspend fun setThemePreference(preference: String)

    /** The locally-stored UI language tag (`en`/`de`/…) or null to follow the device. */
    fun languageOverride(): Flow<String?>
    suspend fun setLanguageOverride(languageTag: String?)

    /** Synchronous read of the theme for the very first frame (before the Flow emits). */
    fun currentThemePreference(): String
}
