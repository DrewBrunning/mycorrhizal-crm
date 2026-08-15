package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.domain.repository.NotificationSettingsRepository
import com.mycorrhizal.crm.domain.repository.WebhookRepository
import com.mycorrhizal.crm.model.network.NotificationConfig
import com.mycorrhizal.crm.model.network.NotificationConfigInput
import com.mycorrhizal.crm.model.network.NotificationTestResult
import com.mycorrhizal.crm.model.network.Webhook
import com.mycorrhizal.crm.model.network.WebhookCreateResponse
import com.mycorrhizal.crm.model.network.WebhookDelivery
import com.mycorrhizal.crm.model.network.WebhookInput
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.toApiError
import javax.inject.Inject
import javax.inject.Singleton

/** M25: webhook CRUD + test + delivery history, thin over [ApiClient]. */
@Singleton
class WebhookRepositoryImpl @Inject constructor(
    private val apiClient: ApiClient,
) : WebhookRepository {

    override suspend fun list(): Result<List<Webhook>> =
        apiClient.listWebhooks().mapError()

    override suspend fun create(input: WebhookInput): Result<WebhookCreateResponse> =
        apiClient.createWebhook(input).mapError()

    override suspend fun update(id: Int, input: WebhookInput): Result<Webhook> =
        apiClient.updateWebhook(id, input).mapError()

    override suspend fun delete(id: Int): Result<Unit> =
        apiClient.deleteWebhook(id).mapError()

    override suspend fun test(id: Int): Result<WebhookDelivery> =
        apiClient.testWebhook(id).mapError()

    override suspend fun deliveries(id: Int): Result<List<WebhookDelivery>> =
        apiClient.getWebhookDeliveries(id).mapError()

    private fun <T> Result<T>.mapError(): Result<T> =
        fold(onSuccess = { Result.success(it) }, onFailure = { Result.failure(it.toApiError()) })
}

/** M25: ntfy/Gotify channel config + per-channel test, thin over [ApiClient]. */
@Singleton
class NotificationSettingsRepositoryImpl @Inject constructor(
    private val apiClient: ApiClient,
) : NotificationSettingsRepository {

    override suspend fun config(): Result<NotificationConfig> =
        apiClient.getNotificationConfig().mapError()

    override suspend fun save(input: NotificationConfigInput): Result<NotificationConfig> =
        apiClient.saveNotificationConfig(input).mapError()

    override suspend fun test(channel: String): Result<NotificationTestResult> =
        apiClient.testNotificationChannel(channel).mapError()

    private fun <T> Result<T>.mapError(): Result<T> =
        fold(onSuccess = { Result.success(it) }, onFailure = { Result.failure(it.toApiError()) })
}
