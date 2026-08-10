package com.mycorrhizal.crm.feature.contacts

import android.content.Intent
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
class MobileLinkRegistryTest {

    @Test
    fun `signal exposes message voice and video actions`() {
        val signal = MobileLinkRegistry.forProtocol("signal")
        assertEquals("signal", signal?.protocol)
        assertEquals(
            listOf(
                MobileActionKind.MESSAGE,
                MobileActionKind.VOICE_CALL,
                MobileActionKind.VIDEO_CALL,
            ),
            signal?.actions?.map { it.kind },
        )
    }

    @Test
    fun `whatsapp exposes message voice and video actions`() {
        val whatsapp = MobileLinkRegistry.forProtocol("whatsapp")
        assertEquals("whatsapp", whatsapp?.protocol)
        assertEquals(
            listOf(
                MobileActionKind.MESSAGE,
                MobileActionKind.VOICE_CALL,
                MobileActionKind.VIDEO_CALL,
            ),
            whatsapp?.actions?.map { it.kind },
        )
    }

    @Test
    fun `protocol lookup is case insensitive`() {
        assertEquals("signal", MobileLinkRegistry.forProtocol("Signal")?.protocol)
        assertEquals("telegram", MobileLinkRegistry.forProtocol("TELEGRAM")?.protocol)
    }

    @Test
    fun `unknown protocol resolves to null`() {
        assertNull(MobileLinkRegistry.forProtocol("matrix"))
        assertNull(MobileLinkRegistry.forProtocol(null))
    }

    @Test
    fun `whatsapp message intent targets the whatsapp package`() {
        val action = MobileLinkRegistry.forProtocol("whatsapp")!!.actions.first()
        val intent = action.intentBuilder("+1-555-0100")
        assertEquals(Intent.ACTION_VIEW, intent.action)
        assertEquals("com.whatsapp", intent.`package`)
        assertEquals("wa.me", intent.data?.host)
        assertEquals("/+15550100", intent.data?.path)
    }

    @Test
    fun `signal message intent uses smsto scheme against the signal package`() {
        val action = MobileLinkRegistry.forProtocol("signal")!!.actions.first()
        val intent = action.intentBuilder("alice.42")
        assertEquals(Intent.ACTION_SENDTO, intent.action)
        assertEquals("smsto", intent.data?.scheme)
        assertEquals("org.thoughtcrime.securesms", intent.`package`)
    }

    @Test
    fun `all seeded protocols are known`() {
        assertEquals(
            setOf("signal", "whatsapp", "telegram", "google-meet", "zoom", "discord"),
            MobileLinkRegistry.all.map { it.protocol }.toSet(),
        )
    }

    @Test
    fun `every action declares at least one mime type`() {
        MobileLinkRegistry.all.forEach { linkType ->
            linkType.actions.forEach { action ->
                assertTrue(
                    "action '${action.label}' on '${linkType.protocol}' needs a MIMETYPE",
                    action.mimeTypes.isNotEmpty(),
                )
            }
        }
    }
}
