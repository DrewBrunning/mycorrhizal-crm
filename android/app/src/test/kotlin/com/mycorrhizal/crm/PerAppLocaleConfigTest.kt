package com.mycorrhizal.crm

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import org.w3c.dom.Element
import java.io.File
import javax.xml.parsers.DocumentBuilderFactory

/**
 * Issue #1921: Android 13+ only offers the Settings -> Apps -> Mycorrhizal ->
 * Language entry when the manifest declares android:localeConfig and the
 * referenced file lists the shipped translations. Pin both, and pin the list to
 * the same five locales the resource folders (and the in-app picker) support.
 */
class PerAppLocaleConfigTest {

    private val supportedLocales = listOf("en", "de", "es", "fr", "it")

    @Test
    fun `the manifest points the OS at the locale config`() {
        val manifest = File("src/main/AndroidManifest.xml").readText()
        assertTrue(
            "AndroidManifest.xml must declare android:localeConfig for the API 33+ per-app language picker",
            manifest.contains("""android:localeConfig="@xml/locales_config""""),
        )
    }

    @Test
    fun `the locale config lists exactly the shipped locales`() {
        val doc = DocumentBuilderFactory.newInstance()
            .newDocumentBuilder()
            .parse(File("src/main/res/xml/locales_config.xml"))
        val nodes = doc.getElementsByTagName("locale")
        val declared = (0 until nodes.length)
            .map { (nodes.item(it) as Element).getAttribute("android:name") }
            .toSet()

        assertEquals(supportedLocales.toSet(), declared)
    }
}
