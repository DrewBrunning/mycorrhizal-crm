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
}
