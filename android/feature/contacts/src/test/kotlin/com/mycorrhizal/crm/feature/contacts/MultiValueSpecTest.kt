package com.mycorrhizal.crm.feature.contacts

import com.mycorrhizal.crm.model.network.Email
import com.mycorrhizal.crm.model.network.OnlineService
import com.mycorrhizal.crm.model.network.Phone
import com.mycorrhizal.crm.ui.components.EmailSpec
import com.mycorrhizal.crm.ui.components.MultiValueSpec
import com.mycorrhizal.crm.ui.components.OnlineServiceSpec
import com.mycorrhizal.crm.ui.components.PhoneSpec
import com.mycorrhizal.crm.ui.components.applyPrefToggle
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/**
 * M7 test cases 1–4 at the spec level: one editor serves Email/Phone/
 * OnlineService, and the guarantee the whole `withValue → copy()` interface
 * exists to provide — `id` and every unshown field survive a load → edit →
 * save round-trip, new rows get their default type, and loaded rows keep
 * theirs — is asserted for each spec rather than for one editor in the
 * abstract (the ticket explicitly demands the tests be parameterized).
 */
class MultiValueSpecTest {

    /** test case 2: pref exclusivity is list-level editor logic, not per-spec. */
    private fun <T> assertPrefExclusive(spec: MultiValueSpec<T>, first: T, second: T, third: T) {
        val toggled = applyPrefToggle(listOf(first, second, third), 1, spec)
        assertNull("the previously-preferred row must be cleared", spec.pref(toggled[0]))
        assertEquals(1, spec.pref(toggled[1]))
        assertNull("unrelated rows must stay un-preferred", spec.pref(toggled[2]))

        // Toggling the already-preferred row back off clears it.
        val toggledOff = applyPrefToggle(toggled, 1, spec)
        assertNull(spec.pref(toggledOff[1]))
    }

    @Test
    fun `email edits preserve id pref and label, write type to contexts, new rows have no default`() {
        val loaded = Email(id = "e1", address = "a@b.c", contexts = listOf("work"), pref = 1, label = "custom")
        val edited = EmailSpec.withType(EmailSpec.withValue(loaded, "x@y.z"), "home")
        // The type writes to contexts[0] (web parity); label is X-ABLabel, preserve-only.
        assertEquals(
            Email(id = "e1", address = "x@y.z", contexts = listOf("home"), pref = 1, label = "custom"),
            edited,
        )
        // A loaded row's type is never overwritten by a default (test case 3).
        assertEquals("work", EmailSpec.type(loaded))
        // New emails carry no default type (test case 3).
        assertNull(EmailSpec.blank().contexts)

        assertPrefExclusive(
            EmailSpec,
            Email(id = "e1", address = "a@b.c", pref = 1),
            Email(address = "b@b.b"),
            Email(address = "c@c.c"),
        )
    }

    @Test
    fun `phone edits preserve id pref features and label, write type to contexts, new rows default to cell`() {
        val loaded = Phone(id = "p1", number = "+1", contexts = listOf("work"), pref = 1, features = listOf("voice"), label = "custom")
        val edited = PhoneSpec.withType(PhoneSpec.withValue(loaded, "+2"), "home")
        assertEquals(
            Phone(id = "p1", number = "+2", contexts = listOf("home"), pref = 1, features = listOf("voice"), label = "custom"),
            edited,
        )
        // A phone's type prefers features[0] (web parity), for a vCard-imported row.
        assertEquals("voice", PhoneSpec.type(loaded))
        // Without features, the type falls back to contexts[0].
        assertEquals("work", PhoneSpec.type(Phone(number = "+1", contexts = listOf("work"))))
        // T81/M7: `cell` lives in blank() — the single place a phone is genuinely new —
        // and now lands in `contexts` so the web's phoneHasToken sees it too.
        assertEquals(listOf("cell"), PhoneSpec.blank().contexts)
        assertNull(PhoneSpec.blank().id)

        assertPrefExclusive(
            PhoneSpec,
            Phone(id = "p1", number = "+1", pref = 1),
            Phone(number = "+2"),
            Phone(number = "+3"),
        )
    }

    @Test
    fun `online service edits preserve service id pref and label, user moves to uri, service name editable`() {
        val loaded = OnlineService(id = "s1", service = "Signal", user = "@dana", contexts = listOf("work"), pref = 1, label = "custom")
        val edited = OnlineServiceSpec.withType(OnlineServiceSpec.withValue(loaded, "@dana2"), "home")
        assertEquals(
            OnlineService(id = "s1", service = "Signal", uri = "@dana2", user = null, contexts = listOf("home"), pref = 1, label = "custom"),
            edited,
        )
        // The service name is editable separately (the link-resolver key).
        assertEquals(
            OnlineService(id = "s1", service = "Telegram", uri = "@dana2", user = null, contexts = listOf("home"), pref = 1, label = "custom"),
            OnlineServiceSpec.withServiceName(edited, "Telegram"),
        )
        // A row stored in `uri` stays in `uri` (no user-move).
        val uriRow = OnlineService(id = "s2", service = "Mastodon", uri = "https://mastodon.example/@dana")
        assertEquals(
            OnlineService(id = "s2", service = "Mastodon", uri = "https://mastodon.example/@dana2"),
            OnlineServiceSpec.withValue(uriRow, "https://mastodon.example/@dana2"),
        )
        assertEquals("work", OnlineServiceSpec.type(loaded))
        assertNull(OnlineServiceSpec.blank().contexts)

        assertPrefExclusive(
            OnlineServiceSpec,
            OnlineService(id = "s1", uri = "x", pref = 1),
            OnlineService(uri = "y"),
            OnlineService(uri = "z"),
        )
    }
}
