package com.mycorrhizal.crm.model.registry

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** Issue #1738: dislike_activity is its own overview section, with no level. */
class PreferenceRegistryTest {

    @Test
    fun `dislike_activity is a free-solo Dislikes category with no proficiency level`() {
        val config = PreferenceCategory.CONFIG.first { it.category == "dislike_activity" }
        assertEquals(PreferenceSection.DISLIKES, config.section)
        assertEquals(PreferenceKeyMode.FREE_SOLO, config.keyMode)
        assertFalse(config.supportsLevel)
        assertFalse(PreferenceCategory.supportsLevelFor("dislike_activity"))
        assertTrue(config.keySuggestions.isEmpty())
    }

    @Test
    fun `the Dislikes section is offered on the overview tab, not the gifts tab`() {
        assertTrue(PreferenceSection.OVERVIEW_TAB.contains(PreferenceSection.DISLIKES))
        assertFalse(PreferenceSection.GIFTS_TAB.contains(PreferenceSection.DISLIKES))
        assertFalse(PreferenceCategory.isGiftsTabCategory("dislike_activity"))
    }

    @Test
    fun `the deprecated hobby category still exists for the 1_x window`() {
        val config = PreferenceCategory.CONFIG.first { it.category == "hobby" }
        assertEquals(PreferenceSection.HOBBY, config.section)
        assertTrue(config.supportsLevel)
        assertTrue(PreferenceSection.OVERVIEW_TAB.contains(PreferenceSection.HOBBY))
    }

    /** Issue #2445 (web #2194/#2430): catch-all categories mirror PREFERENCE_CATEGORY_CONFIG. */
    @Test
    fun `other categories are configured in their sections with disposition keys`() {
        val expected = mapOf(
            "media_other" to Pair(PreferenceSection.MEDIA, listOf("favorite", "like", "dislike")),
            "jewelry_other" to Pair(
                PreferenceSection.JEWELRY,
                listOf("favorite", "like", "dislike", "allergy")
            ),
            "gift_other" to Pair(
                PreferenceSection.GIFT_PREFERENCES,
                listOf("favorite", "like", "dislike", "allergy")
            )
        )
        expected.forEach { (category, want) ->
            val config = PreferenceCategory.CONFIG.single { it.category == category }
            assertEquals(category, want.first, config.section)
            assertEquals(category, PreferenceKeyMode.DISPOSITION, config.keyMode)
            assertEquals(category, want.second, config.keySuggestions)
            assertFalse(category, config.supportsLevel)
        }
    }

    @Test
    fun `jewelry_other and gift_other surface on the gifts tab, media_other does not`() {
        assertTrue(PreferenceCategory.isGiftsTabCategory("jewelry_other"))
        assertTrue(PreferenceCategory.isGiftsTabCategory("gift_other"))
        assertFalse(PreferenceCategory.isGiftsTabCategory("media_other"))
        assertEquals(PreferenceSection.MEDIA, PreferenceCategory.sectionOf("media_other"))
    }
}
