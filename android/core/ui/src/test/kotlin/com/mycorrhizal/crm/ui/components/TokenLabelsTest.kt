package com.mycorrhizal.crm.ui.components

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.model.network.RelationshipEdgeTypes
import com.mycorrhizal.crm.model.registry.CadenceQualifyingType
import com.mycorrhizal.crm.model.registry.LifeEventTypes
import com.mycorrhizal.crm.model.registry.PhoneFeature
import java.io.File
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

/**
 * Issue #1755: every backend token has a translated label.
 *
 * Token source choice: the sets are read straight from the backend Go sources in the repo
 * (the same checkout CI builds), rather than a second hand-copied list, so a token added
 * backend-side fails this test until Android labels it (CLAUDE.md frontend trap 4). The
 * Android build cannot read `backend/openapi.yaml` enums for these (the relationship,
 * life-event and activity-type sets are not enums in the spec), hence the Go sources.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
class TokenLabelsTest {

    private fun repoRoot(): File {
        var dir: File? = File(System.getProperty("user.dir")).absoluteFile
        while (dir != null) {
            if (File(dir, "backend/models/activity.go").isFile) return dir
            dir = dir.parentFile
        }
        error("backend/ sources not found above ${System.getProperty("user.dir")}")
    }

    private fun backend(path: String): String = File(repoRoot(), "backend/$path").readText()

    private fun relationshipTokensFromBackend(): List<String> {
        val src = backend("models/relationship_type_registry.go")
        val body = src.substringAfter("var relationTypeRegistry = map[string]relationTypeDef{")
            .substringBefore("\n}\n")
        return Regex("(?m)^\t\"([a-z_]+)\": \\{").findAll(body).map { it.groupValues[1] }.toList()
    }

    private fun constTokens(file: String, prefix: String): List<String> =
        Regex("$prefix\\w*\\s*=\\s*\"([a-z_]+)\"").findAll(backend(file)).map { it.groupValues[1] }.toList()

    @Test
    fun `backend token parsing finds the expected sets`() {
        // Guards the regexes themselves: a refactor of the Go files must not turn the
        // coverage tests below into vacuous passes.
        assertTrue(relationshipTokensFromBackend().size >= 16)
        assertTrue(constTokens("models/life_event.go", "LifeEventType").size >= 45)
        assertTrue(constTokens("models/activity.go", "InteractionType").size >= 8)
    }

    @Test
    fun `every backend relationship type has a role label`() {
        val missing = relationshipTokensFromBackend().filter { relationshipRoleLabelRes(it) == null }
        assertEquals("relationship tokens without a label", emptyList<String>(), missing)
        // And the Android mirror agrees with the backend registry.
        assertEquals(relationshipTokensFromBackend().toSet(), RelationshipEdgeTypes.ALL.keys)
    }

    @Test
    fun `every backend life event type has a label`() {
        val tokens = constTokens("models/life_event.go", "LifeEventType")
        assertEquals(emptyList<String>(), tokens.filter { lifeEventTypeLabelRes(it) == null })
        val suggested = LifeEventTypes.BY_CATEGORY.values.flatten()
        assertEquals(emptyList<String>(), suggested.filter { lifeEventTypeLabelRes(it) == null })
    }

    @Test
    fun `every backend activity type has a label`() {
        val tokens = constTokens("models/activity.go", "InteractionType")
        assertEquals(emptyList<String>(), tokens.filter { activityTypeLabelRes(it) == null })
        assertEquals(emptyList<String>(), CadenceQualifyingType.ALL.filter { activityTypeLabelRes(it) == null })
    }

    @Test
    fun `every phone feature and contact type option has a label`() {
        assertEquals(emptyList<String>(), PhoneFeature.ALL.filter { typeOptionLabelRes(it) == null })
        assertEquals(emptyList<String>(), CONTACT_TYPE_OPTIONS.filter { typeOptionLabelRes(it) == null })
        assertEquals(emptyList<String>(), CONTEXT_OPTIONS.filter { typeOptionLabelRes(it) == null })
        assertEquals(emptyList<String>(), PERSONAL_INFO_KIND_OPTIONS.filter { typeOptionLabelRes(it) == null })
        assertEquals(emptyList<String>(), ANNIVERSARY_KIND_OPTIONS.filter { typeOptionLabelRes(it) == null })
    }

    @Test
    fun `every backend gift status has a label`() {
        val m = Regex("oneof=(idea purchased given received)").find(backend("models/gift.go"))
        assertNotNull(m)
        assertEquals(emptyList<String>(), m!!.groupValues[1].split(" ").filter { giftStatusLabelRes(it) == null })
    }

    @Test
    fun `humanize falls back to sentence case and keeps custom text`() {
        assertEquals("Job change", humanizeToken("job_change"))
        assertEquals("Main number", humanizeToken("main-number"))
        assertEquals("Mom's cell", humanizeToken("Mom's cell"))
        assertEquals("Brand new type", humanizeToken("  brand_new_type "))
        assertEquals("", humanizeToken(""))
    }

    @Test
    fun `unknown tokens have no resource so the composable wrappers humanize them`() {
        assertEquals(null, lifeEventTypeLabelRes("something_new"))
        assertEquals(null, relationshipRoleLabelRes("godparent_of"))
        assertEquals(null, activityTypeLabelRes("carrier_pigeon"))
        assertEquals(null, typeOptionLabelRes("satellite"))
    }

    @Test
    fun `lookup is case and whitespace tolerant`() {
        assertEquals(typeOptionLabelRes("cell"), typeOptionLabelRes(" Cell "))
    }

    @Test
    fun `the cell token reads Mobile like the edit form dropdown`() {
        val ctx = ApplicationProvider.getApplicationContext<Context>()
        assertEquals("Mobile", ctx.getString(typeOptionLabelRes("cell")!!))
        assertEquals("Partner", ctx.getString(relationshipRoleLabelRes("partner_of")!!))
        assertEquals("Started a new job", lifeEventTypeLabelText(ctx, "job_change"))
        assertEquals("Meal", ctx.getString(activityTypeLabelRes("meal")!!))
    }

    @Test
    @Config(sdk = [35], qualifiers = "de")
    fun `labels are translated in German`() {
        val ctx = ApplicationProvider.getApplicationContext<Context>()
        assertEquals("Mobil", ctx.getString(typeOptionLabelRes("cell")!!))
        assertEquals("Neuen Job begonnen", lifeEventTypeLabelText(ctx, "job_change"))
        assertEquals("Essen", ctx.getString(activityTypeLabelRes("meal")!!))
        assertNotEquals("Partner of", ctx.getString(relationshipRoleLabelRes("partner_of")!!))
        assertEquals("Elternteil", ctx.getString(relationshipRoleLabelRes("parent_of")!!))
        assertEquals("Hobby", ctx.getString(typeOptionLabelRes("hobby")!!))
    }
}
