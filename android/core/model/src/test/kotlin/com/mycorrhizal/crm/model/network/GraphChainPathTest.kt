package com.mycorrhizal.crm.model.network

import org.junit.Assert.assertEquals
import org.junit.Test

/** Issue #1755: the chain path takes caller-supplied (translated) relation labels. */
class GraphChainPathTest {
    private val chain = GraphChain(
        targetName = "Bob",
        steps = listOf(
            GraphChainStep(contactName = "Sister", relation = "sibling_of"),
            GraphChainStep(contactName = "", relation = "spouse_of"),
        ),
    )

    @Test
    fun `default path humanizes underscores`() {
        assertEquals("Sister (sibling of) → Unknown (spouse of)", chain.readablePath)
    }

    @Test
    fun `path uses the supplied labels in step order`() {
        assertEquals("Sister (Geschwister) → Unknown (Ehepartner)", chain.readablePath(listOf("Geschwister", "Ehepartner")))
    }

    @Test
    fun `a missing label falls back to the humanized relation`() {
        assertEquals("Sister (Sibling) → Unknown (spouse of)", chain.readablePath(listOf("Sibling")))
    }

    @Test
    fun `no steps gives an empty path`() {
        assertEquals("", GraphChain().readablePath)
    }

    // Issue #1763: network rows must not repeat the target name in their subtitle.
    @Test
    fun `target-facing path omits the repeated target name on a direct connection`() {
        val direct = GraphChain(
            targetName = "Bea Fontaine",
            steps = listOf(GraphChainStep(contactName = "Bea Fontaine", relation = "related_to")),
        )

        assertEquals("related to", direct.readablePathToTarget(emptyList()))
    }

    @Test
    fun `target-facing path keeps intermediate names and ends with the relationship label`() {
        assertEquals(
            "Sister (Geschwister) → Ehepartner",
            chain.readablePathToTarget(listOf("Geschwister", "Ehepartner")),
        )
    }

    @Test
    fun `target-facing path has no path when there are no steps`() {
        assertEquals("", GraphChain(targetName = "Bea").readablePathToTarget(emptyList()))
    }
}
