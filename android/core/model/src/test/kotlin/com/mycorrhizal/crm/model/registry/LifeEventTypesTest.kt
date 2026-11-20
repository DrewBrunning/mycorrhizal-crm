package com.mycorrhizal.crm.model.registry

import java.io.File
import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * Cross-language parity for the hand-mirrored life-event type registry
 * (frontend trap #4). The backend's life_event_typesByCategoryOrder is the
 * authority; a token missing here becomes a free-text "custom" event on
 * Android even though the server and web already know it (moved_out, #1233).
 */
class LifeEventTypesTest {

    @Test
    fun `suggested tokens match the backend registry`() {
        val registrySource = File("../../../backend/models/life_event_type_registry.go").readText()
        val constantsSource = File("../../../backend/models/life_event.go").readText()
        val constants = Regex("(LifeEventType\\w+)\\s*=\\s*\"([^\"]+)\"")
            .findAll(constantsSource)
            .associate { it.groupValues[1] to it.groupValues[2] }
        val body = Regex(
            """var lifeEventTypesByCategoryOrder = map\[string]\[\]string\{([\s\S]*?)\n}"""
        )
            .find(registrySource)
            ?.groupValues
            ?.get(1)
            ?: error("could not find lifeEventTypesByCategoryOrder in the backend registry")
        val backendTokens = Regex("""LifeEventType\w+""")
            .findAll(body)
            .map { constants[it.value] ?: error("unknown Go life-event constant ${it.value}") }
            .toSet()

        assertEquals(backendTokens, LifeEventTypes.BY_CATEGORY.values.flatten().toSet())
    }
}
