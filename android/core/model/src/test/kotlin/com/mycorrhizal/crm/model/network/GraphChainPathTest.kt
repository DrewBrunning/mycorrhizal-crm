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
}
