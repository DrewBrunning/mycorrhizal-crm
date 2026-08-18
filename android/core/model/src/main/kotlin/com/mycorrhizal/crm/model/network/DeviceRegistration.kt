package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

/**
 * M5 §5a (issue #152) — mobile push device registrations. Mirrors
 * `backend/models/notification.go` by hand: a device is one platform push
 * token (`client` identifies the platform — `fcm` today, `apns` accepted but
 * undelivered server-side), registered against `POST /notifications/devices`
 * and removed with `DELETE /notifications/devices/:id`. The web app only views
 * and deletes devices; Android is the only client that registers them.
 */

/** POST /notifications/devices request body — `{ token, client, device_label }`. */
@JsonClass(generateAdapter = true)
data class DeviceRegistrationInput(
    val token: String,
    val client: String,
    @Json(name = "device_label") val deviceLabel: String = "",
)

/**
 * A registered device — the raw `DeviceRegistration` row the API returns
 * (201 on create, and inside GET /notifications/devices' `{ devices: [...] }`).
 * The server reassigns an existing (client, token) row to the new owner on
 * re-registration, so a device re-registers on every launch without fanning out.
 */
@JsonClass(generateAdapter = true)
data class DeviceRegistration(
    val id: Int = 0,
    @Json(name = "created_at") val createdAt: String? = null,
    @Json(name = "updated_at") val updatedAt: String? = null,
    val token: String = "",
    val client: String = "",
    @Json(name = "device_label") val deviceLabel: String = "",
)

/** GET /notifications/devices — `{ devices: [...] }`. */
@JsonClass(generateAdapter = true)
data class DeviceRegistrationsResponse(
    val devices: List<DeviceRegistration> = emptyList(),
)
