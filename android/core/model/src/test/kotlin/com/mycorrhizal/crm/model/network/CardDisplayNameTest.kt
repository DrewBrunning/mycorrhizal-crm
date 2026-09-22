package com.mycorrhizal.crm.model.network

import org.junit.Assert.assertEquals
import org.junit.Test

class CardDisplayNameTest {

    private fun cardWith(components: List<NameComponent>) = Card(name = Name(components = components))

    @Test
    fun `joins given and surname when no other components are present`() {
        val card = cardWith(
            listOf(
                NameComponent(kind = "given", value = "Dana"),
                NameComponent(kind = "surname", value = "White"),
            ),
        )
        assertEquals("Dana White", card.displayName)
    }

    @Test
    fun `issue 832 - includes prefix middle and suffix, matching web's getContactDisplayName`() {
        val card = cardWith(
            listOf(
                NameComponent(kind = "title", value = "Dr."),
                NameComponent(kind = "given", value = "Dana"),
                NameComponent(kind = "given2", value = "Marie"),
                NameComponent(kind = "surname", value = "White"),
                NameComponent(kind = "generation", value = "Jr."),
            ),
        )
        assertEquals("Dr. Dana Marie White Jr.", card.displayName)
    }

    @Test
    fun `omits a name component that is absent rather than leaving a gap`() {
        val card = cardWith(
            listOf(
                NameComponent(kind = "given", value = "Dana"),
                NameComponent(kind = "surname", value = "White"),
                NameComponent(kind = "generation", value = "Jr."),
            ),
        )
        assertEquals("Dana White Jr.", card.displayName)
    }

    @Test
    fun `falls back to name full when there are no name components`() {
        val card = Card(name = Name(full = "The Dude"))
        assertEquals("The Dude", card.displayName)
    }

    @Test
    fun `falls back to a literal Contact when nothing at all is present`() {
        assertEquals("Contact", Card().displayName)
    }
}
