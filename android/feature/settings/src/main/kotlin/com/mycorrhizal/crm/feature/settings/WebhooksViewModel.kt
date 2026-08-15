package com.mycorrhizal.crm.feature.settings

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.WebhookRepository
import com.mycorrhizal.crm.model.network.Webhook
import com.mycorrhizal.crm.model.network.WebhookCreateResponse
import com.mycorrhizal.crm.model.network.WebhookDelivery
import com.mycorrhizal.crm.model.network.WebhookInput
import com.mycorrhizal.crm.network.ApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

data class WebhooksUiState(
    val webhooks: List<Webhook> = emptyList(),
    val isLoading: Boolean = false,
    val isSaving: Boolean = false,
    /** Ids currently running a test. */
    val testingIds: Set<Int> = emptySet(),
    /** Ids whose delivery history is expanded. */
    val expandedIds: Set<Int> = emptySet(),
    /** Loaded delivery history per webhook id. */
    val deliveries: Map<Int, List<WebhookDelivery>> = emptyMap(),
    /** The just-created webhook whose secret is on screen (one-shot dialog). */
    val createdWebhook: WebhookCreateResponse? = null,
    /** A transient action error (test/delete/save/load), shown and then cleared. */
    val error: String? = null,
    /** A transient action success message (test). */
    val message: String? = null,
)

@HiltViewModel
class WebhooksViewModel @Inject constructor(
    private val repository: WebhookRepository,
) : ViewModel() {

    private val _uiState = MutableStateFlow(WebhooksUiState())
    val uiState: StateFlow<WebhooksUiState> = _uiState.asStateFlow()

    init {
        load()
    }

    fun load() {
        if (_uiState.value.isLoading) return
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null, message = null) }
            repository.list()
                .onSuccess { list ->
                    _uiState.update { it.copy(isLoading = false, webhooks = list) }
                }
                .onFailure { e ->
                    _uiState.update { it.copy(isLoading = false, error = e.displayMessage()) }
                }
        }
    }

    fun save(input: WebhookInput, editingId: Int?) {
        if (_uiState.value.isSaving) return
        viewModelScope.launch {
            _uiState.update { it.copy(isSaving = true, error = null, message = null) }
            if (editingId == null) {
                repository.create(input)
                    .onSuccess { created ->
                        _uiState.update {
                            it.copy(
                                isSaving = false,
                                webhooks = listOf(created.toWebhook()) + it.webhooks,
                                createdWebhook = created,
                            )
                        }
                    }
                    .onFailure { e ->
                        _uiState.update { it.copy(isSaving = false, error = e.displayMessage()) }
                    }
            } else {
                repository.update(editingId, input)
                    .onSuccess { updated ->
                        _uiState.update {
                            it.copy(
                                isSaving = false,
                                webhooks = it.webhooks.map { w -> if (w.id == editingId) updated else w },
                            )
                        }
                    }
                    .onFailure { e ->
                        _uiState.update { it.copy(isSaving = false, error = e.displayMessage()) }
                    }
            }
        }
    }

    fun delete(webhook: Webhook) {
        viewModelScope.launch {
            _uiState.update { it.copy(error = null, message = null) }
            repository.delete(webhook.id)
                .onSuccess {
                    _uiState.update {
                        it.copy(
                            webhooks = it.webhooks.filterNot { w -> w.id == webhook.id },
                            deliveries = it.deliveries - webhook.id,
                            expandedIds = it.expandedIds - webhook.id,
                        )
                    }
                }
                .onFailure { e ->
                    _uiState.update { it.copy(error = e.displayMessage()) }
                }
        }
    }

    fun test(webhook: Webhook) {
        viewModelScope.launch {
            _uiState.update {
                it.copy(testingIds = it.testingIds + webhook.id, error = null, message = null)
            }
            repository.test(webhook.id)
                .onSuccess { delivery ->
                    val success = delivery.statusCode?.let { it in 200..299 } == true
                    _uiState.update {
                        it.copy(
                            testingIds = it.testingIds - webhook.id,
                            deliveries = it.deliveries + (webhook.id to listOf(delivery) + it.deliveries[webhook.id].orEmpty()),
                            expandedIds = it.expandedIds + webhook.id,
                            message = if (success) delivery.statusCode?.toString() else null,
                            error = if (success) null else delivery.error ?: "status ${delivery.statusCode ?: "unknown"}",
                        )
                    }
                }
                .onFailure { e ->
                    _uiState.update { it.copy(testingIds = it.testingIds - webhook.id, error = e.displayMessage()) }
                }
        }
    }

    fun toggleDeliveries(webhookId: Int) {
        val expanded = _uiState.value.expandedIds.contains(webhookId)
        _uiState.update {
            it.copy(
                expandedIds = if (expanded) it.expandedIds - webhookId else it.expandedIds + webhookId,
            )
        }
        if (!expanded && !_uiState.value.deliveries.containsKey(webhookId)) {
            viewModelScope.launch {
                repository.deliveries(webhookId)
                    .onSuccess { list ->
                        _uiState.update { it.copy(deliveries = it.deliveries + (webhookId to list)) }
                    }
                    .onFailure { e ->
                        // Delivery history is a secondary surface — a failed load
                        // shows the row's own inline error, not the whole screen's.
                        _uiState.update { it.copy(error = e.displayMessage()) }
                    }
            }
        }
    }

    fun dismissCreatedWebhook() {
        _uiState.update { it.copy(createdWebhook = null) }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null, message = null) }
    }

    private fun Throwable.displayMessage(): String =
        (this as? ApiError)?.displayMessage ?: message ?: "error"
}

private fun WebhookCreateResponse.toWebhook(): Webhook =
    Webhook(id = id, name = name, url = url, events = events, isActive = isActive, createdAt = createdAt)
