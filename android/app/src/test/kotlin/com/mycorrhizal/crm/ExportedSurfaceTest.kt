package com.mycorrhizal.crm

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test
import org.w3c.dom.Element
import org.xml.sax.InputSource
import java.io.File
import java.io.StringReader
import javax.xml.parsers.DocumentBuilderFactory

// Issue #1357 / ADR 0029: nothing else enumerates the externally-invokable surface of the
// MERGED manifest (app code + every library's contributed manifest). This test scans the
// merged manifest of every shipped flavor x {debug, release} and compares it, exactly, with
// `app/exported-surface-allowlist.txt`: a new exported component or intent-filter fails
// (unlisted), and so does an allowlist row that no longer matches anything (stale). Each row
// carries a reason. Update procedure: android/app/exported-surface-allowlist.txt header.
//
// The manifests and allowlist are handed in by app/build.gradle.kts as system properties, and
// the test tasks depend on the process<Variant>MainManifest tasks, so a manifest is never stale.
class ExportedSurfaceTest {

    @Test
    fun `merged manifest surface of every variant equals the allowlist`() {
        val manifests = System.getProperty("exportedSurface.manifests")
        val allowlist = System.getProperty("exportedSurface.allowlist")
        assertTrue(
            "exportedSurface.* system properties missing: run via Gradle (app/build.gradle.kts wires them)",
            !manifests.isNullOrBlank() && !allowlist.isNullOrBlank(),
        )
        val entries = ExportedSurface.parseAllowlist(File(allowlist).readText())
        val variants = manifests.split(';').filter { it.isNotBlank() }.associate {
            val (name, path) = it.split('=', limit = 2)
            name to File(path).readText()
        }
        assertTrue("expected obtainium/play/foss x debug/release = 6 variants, got ${variants.keys}", variants.size == 6)
        val problems = variants.flatMap { (variant, xml) -> ExportedSurface.diff(variant, xml, entries) }
        if (problems.isNotEmpty()) {
            fail(
                "The merged manifest's externally-invokable surface no longer matches " +
                    "android/app/exported-surface-allowlist.txt (issue #1357). Review each change against " +
                    "ADR 0029; if it is intended, add/remove the row WITH a reason.\n" + problems.joinToString("\n"),
            )
        }
    }

    // --- the checker itself (a gate that has never failed proves nothing) -------------------

    private val ns = "http://schemas.android.com/apk/res/android"
    private fun manifest(body: String) =
        """<manifest xmlns:android="$ns"><application>$body</application></manifest>"""

    private fun entry(key: String, flavors: String = "*", types: String = "*") =
        ExportedSurface.Entry(key, flavors.split(',').toSet(), types.split(',').toSet(), "r")

    @Test
    fun `scan lists exported components and every filter but not private components`() {
        val xml = manifest(
            """
            <activity android:name="a.Main" android:exported="true">
              <intent-filter><action android:name="A"/><category android:name="C"/>
                <data android:scheme="s" android:host="h"/></intent-filter>
            </activity>
            <service android:name="a.Priv" android:exported="false"/>
            <receiver android:name="a.Guarded" android:exported="true" android:permission="p.P"/>
            <receiver android:name="a.Implicit"><intent-filter><action android:name="B"/></intent-filter></receiver>
            <provider android:name="a.Prov"/>
            """,
        )
        assertEquals(
            setOf(
                "component activity a.Main permission=none",
                "component receiver a.Guarded permission=p.P",
                "component receiver a.Implicit permission=none",
                "filter a.Main :: actions=A; categories=C; data=scheme=s host=h",
                "filter a.Implicit :: actions=B; categories=; data=",
            ),
            ExportedSurface.scan(xml),
        )
    }

    @Test
    fun `diff flags an unlisted addition and a stale row and honours flavor and type scope`() {
        val xml = manifest("""<activity android:name="a.X" android:exported="true"/>""")
        val key = "component activity a.X permission=none"
        assertEquals(emptyList<String>(), ExportedSurface.diff("fossDebug", xml, listOf(entry(key))))
        assertEquals(emptyList<String>(), ExportedSurface.diff("fossDebug", xml, listOf(entry(key, "foss", "debug"))))
        val unlisted = ExportedSurface.diff("fossDebug", xml, emptyList())
        assertEquals(1, unlisted.size)
        assertTrue(unlisted[0], unlisted[0].startsWith("UNLISTED"))
        val stale = ExportedSurface.diff("fossDebug", xml, listOf(entry(key), entry("component service gone permission=none")))
        assertEquals(1, stale.size)
        assertTrue(stale[0], stale[0].startsWith("STALE"))
        // A row scoped to another flavor/type must not cover this variant ...
        assertEquals(1, ExportedSurface.diff("fossDebug", xml, listOf(entry(key, "play", "*"))).count { it.startsWith("UNLISTED") })
        assertEquals(1, ExportedSurface.diff("fossDebug", xml, listOf(entry(key, "*", "release"))).count { it.startsWith("UNLISTED") })
        // ... and a row scoped to this variant must go stale when the component is absent from it.
        val none = ExportedSurface.diff("fossDebug", manifest(""), listOf(entry(key, "foss", "debug")))
        assertTrue(none.single().startsWith("STALE"))
    }

    @Test
    fun `allowlist rows must carry all columns and a reason`() {
        assertEquals(1, ExportedSurface.parseAllowlist("# c\n\ncomponent activity a permission=none | * | * | why\n").size)
        listOf(
            "component activity a permission=none | * | *",
            "component activity a permission=none | * | * |  ",
            "component activity a permission=none | * | bogus | why",
        ).forEach { line ->
            try {
                ExportedSurface.parseAllowlist(line)
                fail("should reject: $line")
            } catch (_: IllegalArgumentException) {
                // expected
            }
        }
    }
}

internal object ExportedSurface {
    private const val ANDROID_NS = "http://schemas.android.com/apk/res/android"
    private val componentTags = listOf("activity", "activity-alias", "service", "receiver", "provider")

    data class Entry(val key: String, val flavors: Set<String>, val types: Set<String>, val reason: String) {
        fun appliesTo(flavor: String, type: String) =
            (flavors == setOf("*") || flavor in flavors) && (types == setOf("*") || type in types)
    }

    /** `key | flavors | types | reason`; `#` comments and blank lines ignored. */
    fun parseAllowlist(text: String): List<Entry> =
        text.lines().map { it.trim() }.filter { it.isNotEmpty() && !it.startsWith("#") }.map { line ->
            val cols = line.split('|', limit = 4).map { it.trim() }
            require(cols.size == 4 && cols[3].isNotEmpty()) { "allowlist row needs 'key | flavors | types | reason': $line" }
            val types = cols[2].split(',').map { it.trim() }.toSet()
            require(types.all { it == "*" || it == "debug" || it == "release" }) { "bad types column: $line" }
            Entry(cols[0], cols[1].split(',').map { it.trim() }.toSet(), types, cols[3])
        }

    private fun attr(e: Element, name: String): String? =
        e.getAttributeNS(ANDROID_NS, name).takeIf { it.isNotEmpty() }

    /** Keys for every effectively-exported component and every intent-filter in a merged manifest. */
    fun scan(manifestXml: String): Set<String> {
        val factory = DocumentBuilderFactory.newInstance().apply { isNamespaceAware = true }
        val doc = factory.newDocumentBuilder().parse(InputSource(StringReader(manifestXml)))
        val app = doc.getElementsByTagName("application").item(0) as Element
        val keys = sortedSetOf<String>()
        val children = app.childNodes
        for (i in 0 until children.length) {
            val c = children.item(i) as? Element ?: continue
            if (c.tagName !in componentTags) continue
            val name = attr(c, "name") ?: continue
            val filters = c.getElementsByTagName("intent-filter")
            // Effective export: explicit value wins; absent + any intent-filter means exported
            // (the pre-Android-12 default; also the conservative reading here).
            val exported = attr(c, "exported")?.let { it == "true" } ?: (filters.length > 0)
            if (exported) keys += "component ${c.tagName} $name permission=${attr(c, "permission") ?: "none"}"
            for (j in 0 until filters.length) keys += "filter $name :: " + filterSignature(filters.item(j) as Element)
        }
        return keys
    }

    private fun names(f: Element, tag: String): String {
        val l = f.getElementsByTagName(tag)
        return (0 until l.length).map { attr(l.item(it) as Element, "name") ?: "?" }.sorted().joinToString(",")
    }

    private fun filterSignature(f: Element): String {
        val l = f.getElementsByTagName("data")
        val data = (0 until l.length).map { i ->
            val d = l.item(i) as Element
            listOf("scheme", "host", "port", "path", "pathPrefix", "pathPattern", "pathAdvancedPattern", "pathSuffix", "mimeType")
                .mapNotNull { a -> attr(d, a)?.let { "$a=$it" } }.joinToString(" ")
        }.sorted().joinToString(" ; ")
        return "actions=${names(f, "action")}; categories=${names(f, "category")}; data=$data"
    }

    /** Human-readable problems for one variant (`obtainiumDebug`); empty when it matches exactly. */
    fun diff(variant: String, manifestXml: String, entries: List<Entry>): List<String> {
        val type = if (variant.endsWith("Debug")) "debug" else "release"
        val flavor = variant.removeSuffix(if (type == "debug") "Debug" else "Release")
        val observed = scan(manifestXml)
        val listed = entries.filter { it.appliesTo(flavor, type) }.map { it.key }.toSet()
        return (observed - listed).map { "UNLISTED in $variant (add: `$it | $flavor | $type | <reason>`): $it" } +
            (listed - observed).map { "STALE in $variant (row matches nothing; remove or narrow its scope): $it" }
    }
}
