package com.mycorrhizal.crm.feature.contacts

import android.content.Intent
import android.net.Uri

/**
 * Mobile link-action enrichment (ticket §7.6). The server owns the existence
 * of a link ("Alice uses Signal, handle alice.42"); the Android client owns
 * the actions — how to reach the person on that service. Each [MobileLinkType]
 * maps a `LinkFieldType.protocol` to one or more actions, and each action is
 * only shown when the device actually has the app (proven by a
 * [ContactsContract.Data] MIMETYPE row, resolved via [resolveAvailableActions]).
 *
 * This registry is the Android equivalent of the frontend's hardcoded
 * enum-mirror convention: it must stay in sync with the `LinkFieldType`
 * protocols the server seeds (plus any a user's instance creates).
 *
 * MIMETYPE verification status (2026-08-10, Pixel 8a): the Signal, WhatsApp,
 * and Google Meet strings below were read from a real device's
 * `ContactsContract.Data` rows. Telegram/Zoom/Discord are educated guesses
 * from each app's public CONTACT mime-type registration and only appear once
 * a handle is actually saved to a device contact — treat them as unverified.
 */
enum class MobileActionKind { MESSAGE, VOICE_CALL, VIDEO_CALL, APP_OPEN, APP_CALL }

data class MobileLinkAction(
    val label: String,
    val kind: MobileActionKind,
    /** ContactsContract.Data MIMETYPEs that prove the app is installed. */
    val mimeTypes: List<String>,
    val intentBuilder: (handle: String) -> Intent,
)

enum class MobileLinkCategory { MESSAGING, SOCIAL, MEETING, OTHER }

data class MobileLinkType(
    val protocol: String,
    val category: MobileLinkCategory,
    val actions: List<MobileLinkAction>,
)

/**
 * The canonical, seeded registry (ticket §7.6.3). A user-created custom link
 * type (e.g. "matrix") has no entry here and degrades to the plain tappable
 * link + copy from the web app — no actions.
 */
object MobileLinkRegistry {

    private fun sendTo(uri: String, pkg: String? = null) = Intent(Intent.ACTION_SENDTO).apply {
        data = Uri.parse(uri)
        if (pkg != null) `package` = pkg
    }

    private fun view(uri: String, pkg: String? = null) = Intent(Intent.ACTION_VIEW).apply {
        data = Uri.parse(uri)
        if (pkg != null) `package` = pkg
    }

    private val signal = MobileLinkType(
        protocol = "signal",
        category = MobileLinkCategory.MESSAGING,
        actions = listOf(
            MobileLinkAction(
                label = "Message",
                kind = MobileActionKind.MESSAGE,
                mimeTypes = listOf("vnd.android.cursor.item/vnd.org.thoughtcrime.securesms.contact"),
                intentBuilder = { handle -> sendTo("smsto:$handle", "org.thoughtcrime.securesms") },
            ),
            MobileLinkAction(
                label = "Voice Call",
                kind = MobileActionKind.VOICE_CALL,
                mimeTypes = listOf("vnd.android.cursor.item/vnd.org.thoughtcrime.securesms.call"),
                intentBuilder = { handle -> view("sgnl://signal.me/#p/$handle", "org.thoughtcrime.securesms") },
            ),
            MobileLinkAction(
                label = "Video Call",
                kind = MobileActionKind.VIDEO_CALL,
                mimeTypes = listOf("vnd.android.cursor.item/vnd.org.thoughtcrime.securesms.videocall"),
                intentBuilder = { handle -> view("sgnl://video.call?recipient=$handle", "org.thoughtcrime.securesms") },
            ),
        ),
    )

    private val whatsapp = MobileLinkType(
        protocol = "whatsapp",
        category = MobileLinkCategory.MESSAGING,
        actions = listOf(
            MobileLinkAction(
                label = "Message",
                kind = MobileActionKind.MESSAGE,
                mimeTypes = listOf("vnd.android.cursor.item/vnd.com.whatsapp.profile"),
                intentBuilder = { handle ->
                    view("https://wa.me/${handle.filter { it.isDigit() || it == '+' }}", "com.whatsapp")
                },
            ),
            MobileLinkAction(
                label = "Voice Call",
                kind = MobileActionKind.VOICE_CALL,
                mimeTypes = listOf("vnd.android.cursor.item/vnd.com.whatsapp.voip.call"),
                intentBuilder = { handle ->
                    view("https://wa.me/${handle.filter { it.isDigit() || it == '+' }}", "com.whatsapp")
                },
            ),
            MobileLinkAction(
                label = "Video Call",
                kind = MobileActionKind.VIDEO_CALL,
                mimeTypes = listOf("vnd.android.cursor.item/vnd.com.whatsapp.video.call"),
                intentBuilder = { handle ->
                    view("https://wa.me/${handle.filter { it.isDigit() || it == '+' }}", "com.whatsapp")
                },
            ),
        ),
    )

    private val telegram = MobileLinkType(
        protocol = "telegram",
        category = MobileLinkCategory.MESSAGING,
        actions = listOf(
            MobileLinkAction(
                label = "Message",
                kind = MobileActionKind.MESSAGE,
                mimeTypes = listOf("vnd.android.cursor.item/vnd.org.telegram.messenger.android.profile"),
                intentBuilder = { handle -> view("tg://resolve?phone=$handle", "org.telegram.messenger") },
            ),
            MobileLinkAction(
                label = "Voice Call",
                kind = MobileActionKind.VOICE_CALL,
                mimeTypes = listOf("vnd.android.cursor.item/vnd.org.telegram.messenger.android.call"),
                intentBuilder = { handle -> view("tg://call?phone=$handle", "org.telegram.messenger") },
            ),
        ),
    )

    private val googleMeet = MobileLinkType(
        protocol = "google-meet",
        category = MobileLinkCategory.MEETING,
        actions = listOf(
            MobileLinkAction(
                label = "Video Call",
                kind = MobileActionKind.VIDEO_CALL,
                mimeTypes = listOf(
                    // Verified on-device (Pixel 8a): Google Meet registers these
                    // Data MIMETYPEs when a Meet handle is saved to a contact.
                    "vnd.android.cursor.item/com.google.android.apps.tachyon.phone.meet",
                    "vnd.android.cursor.item/com.google.android.apps.tachyon.phone.meet.audio",
                    "vnd.android.cursor.item/com.google.android.apps.tachyon.email.meet",
                ),
                intentBuilder = { handle -> view("https://meet.google.com/$handle") },
            ),
        ),
    )

    private val zoom = MobileLinkType(
        protocol = "zoom",
        category = MobileLinkCategory.MEETING,
        actions = listOf(
            MobileLinkAction(
                label = "Meeting",
                kind = MobileActionKind.VIDEO_CALL,
                mimeTypes = listOf("vnd.android.cursor.item/vnd.com.zoom.video.call"),
                intentBuilder = { handle -> view("zoommtg://zoom.us/start?confno=$handle") },
            ),
        ),
    )

    private val discord = MobileLinkType(
        protocol = "discord",
        category = MobileLinkCategory.SOCIAL,
        actions = listOf(
            MobileLinkAction(
                label = "Message",
                kind = MobileActionKind.MESSAGE,
                mimeTypes = listOf("vnd.android.cursor.item/vnd.com.discord"),
                intentBuilder = { handle -> view("discord://discord.com/users/$handle", "com.discord") },
            ),
            MobileLinkAction(
                label = "Voice Call",
                kind = MobileActionKind.VOICE_CALL,
                mimeTypes = listOf("vnd.android.cursor.item/vnd.com.discord.call"),
                intentBuilder = { handle -> view("discord://discord.com/channels/$handle", "com.discord") },
            ),
        ),
    )

    val all: List<MobileLinkType> = listOf(signal, whatsapp, telegram, googleMeet, zoom, discord)

    fun forProtocol(protocol: String?): MobileLinkType? =
        protocol?.let { p ->
            all.firstOrNull { it.protocol == p } ?: all.firstOrNull { it.protocol.equals(p, ignoreCase = true) }
        }
}
