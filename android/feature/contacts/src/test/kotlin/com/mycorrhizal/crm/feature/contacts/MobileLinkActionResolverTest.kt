package com.mycorrhizal.crm.feature.contacts

import android.content.ContentProvider
import android.content.ContentValues
import android.content.ContextWrapper
import android.database.Cursor
import android.database.MatrixCursor
import android.net.Uri
import android.provider.ContactsContract
import androidx.test.core.app.ApplicationProvider
import kotlinx.coroutines.test.runTest
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.shadows.ShadowContentResolver

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
class MobileLinkActionResolverTest {

    private val provider = FakeContactsProvider()

    private fun contentResolver() =
        (ApplicationProvider.getApplicationContext<android.content.Context>() as ContextWrapper).contentResolver

    private fun setUpProvider(mimeTypes: List<String>) {
        provider.mimeTypes = mimeTypes
        ShadowContentResolver.registerProviderInternal(
            ContactsContract.AUTHORITY,
            provider,
        )
    }

    @After
    fun tearDown() {
        ShadowContentResolver.reset()
    }

    @Test
    fun `returns no actions when device has no matching app`() = runTest {
        setUpProvider(emptyList())
        val resolver = MobileLinkActionResolver(contentResolver())

        val available = resolver.resolveAvailableActions(
            MobileLinkRegistry.forProtocol("signal")!!,
            "alice.42",
        )

        assertEquals(emptyList<MobileLinkAction>(), available)
    }

    @Test
    fun `returns only actions whose app is installed`() = runTest {
        // Signal message app installed; call/video apps not.
        setUpProvider(listOf("vnd.android.cursor.item/vnd.org.thoughtcrime.securesms.contact"))
        val resolver = MobileLinkActionResolver(contentResolver())

        val available = resolver.resolveAvailableActions(
            MobileLinkRegistry.forProtocol("signal")!!,
            "alice.42",
        )

        assertEquals(listOf(MobileActionKind.MESSAGE), available.map { it.kind })
    }

    @Test
    fun `returns all actions when full suite installed`() = runTest {
        setUpProvider(
            listOf(
                "vnd.android.cursor.item/vnd.org.thoughtcrime.securesms.contact",
                "vnd.android.cursor.item/vnd.org.thoughtcrime.securesms.call",
                "vnd.android.cursor.item/vnd.org.thoughtcrime.securesms.videocall",
            ),
        )
        val resolver = MobileLinkActionResolver(contentResolver())

        val available = resolver.resolveAvailableActions(
            MobileLinkRegistry.forProtocol("signal")!!,
            "alice.42",
        )

        assertEquals(
            listOf(MobileActionKind.MESSAGE, MobileActionKind.VOICE_CALL, MobileActionKind.VIDEO_CALL),
            available.map { it.kind },
        )
    }

    @Test
    fun `irrelevant mime types do not surface actions`() = runTest {
        setUpProvider(listOf("vnd.android.cursor.item/vnd.com.slack"))
        val resolver = MobileLinkActionResolver(contentResolver())

        val available = resolver.resolveAvailableActions(
            MobileLinkRegistry.forProtocol("whatsapp")!!,
            "+1-555-0100",
        )

        assertEquals(emptyList<MobileLinkAction>(), available)
    }
}

/** Minimal provider: serves a fixed list of MIMETYPE rows for any Data query. */
private class FakeContactsProvider : ContentProvider() {
    var mimeTypes: List<String> = emptyList()

    override fun onCreate(): Boolean = true

    override fun query(
        uri: Uri,
        projection: Array<out String>?,
        selection: String?,
        selectionArgs: Array<out String>?,
        sortOrder: String?,
    ): Cursor {
        val cursor = MatrixCursor(arrayOf(ContactsContract.Data.MIMETYPE))
        val wanted = selectionArgs?.toSet() ?: emptySet()
        mimeTypes.filter { it in wanted }.forEach { mime ->
            cursor.addRow(arrayOf(mime))
        }
        return cursor
    }

    override fun getType(uri: Uri): String? = null

    override fun insert(uri: Uri, values: ContentValues?): Uri? = null

    override fun delete(uri: Uri, selection: String?, selectionArgs: Array<out String>?): Int = 0

    override fun update(
        uri: Uri,
        values: ContentValues?,
        selection: String?,
        selectionArgs: Array<out String>?,
    ): Int = 0
}
