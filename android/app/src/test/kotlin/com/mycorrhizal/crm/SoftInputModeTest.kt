package com.mycorrhizal.crm

import java.io.File
import javax.xml.parsers.DocumentBuilderFactory
import org.junit.Assert.assertEquals
import org.junit.Test
import org.w3c.dom.Element

/**
 * Issue #2613: with the activity on the default (unspecified) soft-input mode, an edge-to-edge
 * window PANS when the keyboard opens, which scrolled the top app bar away, drew the first field
 * under the status bar and left a blank band above the keyboard. `adjustResize` makes the system
 * deliver the IME insets to Compose instead, so `imePadding()` (the pinned save bar) and the
 * scrolling form content move above the keyboard while the Scaffold's top bar stays put.
 */
class SoftInputModeTest {

    @Test
    fun `MainActivity resizes for the keyboard instead of panning`() {
        val manifest = File("src/main/AndroidManifest.xml")
        val doc = DocumentBuilderFactory.newInstance().apply { isNamespaceAware = true }
            .newDocumentBuilder().parse(manifest)
        val activities = doc.getElementsByTagName("activity")
        val main = (0 until activities.length)
            .map { activities.item(it) as Element }
            .first { it.getAttributeNS(ANDROID_NS, "name") == ".MainActivity" }
        assertEquals("adjustResize", main.getAttributeNS(ANDROID_NS, "windowSoftInputMode"))
    }

    private companion object {
        const val ANDROID_NS = "http://schemas.android.com/apk/res/android"
    }
}
