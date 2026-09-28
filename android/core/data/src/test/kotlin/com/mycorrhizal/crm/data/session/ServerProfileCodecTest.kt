package com.mycorrhizal.crm.data.session

import com.mycorrhizal.crm.domain.profile.ServerProfile
import com.mycorrhizal.crm.domain.profile.ServerProfileKind
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * ADR 0028 Decision 1: the profile list is persisted in one DataStore string
 * slot. The codec must round-trip labels/URLs with awkward characters (tabs,
 * newlines, `&`, `=`) and must degrade on a corrupt row rather than throw.
 */
class ServerProfileCodecTest {

    private fun remote(id: String, url: String, label: String) =
        ServerProfile(id, ServerProfileKind.Remote(url), label)

    @Test
    fun `round-trips remote and local profiles with their active id`() {
        val snapshot = ProfilesSnapshot(
            profiles = listOf(
                remote("p1", "https://one.example.com", "One"),
                ServerProfile("p2", ServerProfileKind.Local, "On this device"),
            ),
            activeProfileId = "p2",
        )

        val decoded = ServerProfileCodec.decode(ServerProfileCodec.encode(snapshot), "p2")

        assertEquals(snapshot, decoded)
    }

    @Test
    fun `round-trips labels and urls containing separators and unicode`() {
        val snapshot = ProfilesSnapshot(
            profiles = listOf(
                remote("p1", "https://one.example.com/path?a=1&b=2", "Wörk\tand\nnewline"),
            ),
            activeProfileId = "p1",
        )

        val decoded = ServerProfileCodec.decode(ServerProfileCodec.encode(snapshot), "p1")

        assertEquals(snapshot, decoded)
    }

    @Test
    fun `null raw decodes to an empty snapshot`() {
        assertEquals(ProfilesSnapshot(), ServerProfileCodec.decode(null, "p1"))
    }

    @Test
    fun `malformed rows are dropped rather than throwing`() {
        val raw = "not-enough-columns\tremote\tx\n" +
            "p2\tunknownkind\thttps%3A%2F%2Ftwo.example\tTwo\n" +
            "\tremote\thttps%3A%2F%2Fthree.example\tThree\n" +
            "p4\tremote\thttps%3A%2F%2Ffour.example\tFour"

        val decoded = ServerProfileCodec.decode(raw, "p2")

        // Only the one well-formed row survives; the malformed ones are dropped.
        assertEquals(1, decoded.profiles.size)
        assertEquals("p4", decoded.profiles.single().id)
    }

    @Test
    fun `active id falls back to the first profile when the stored one is unknown`() {
        val snapshot = ProfilesSnapshot(
            profiles = listOf(remote("p1", "https://one.example.com", "One")),
            activeProfileId = "p1",
        )

        val decoded = ServerProfileCodec.decode(ServerProfileCodec.encode(snapshot), "missing")

        assertEquals("p1", decoded.activeProfileId)
    }

    @Test
    fun `active id is null when there are no profiles`() {
        assertNull(ServerProfileCodec.decode("", "p1").activeProfileId)
        assertTrue(ServerProfileCodec.decode("", "p1").profiles.isEmpty())
    }
}
