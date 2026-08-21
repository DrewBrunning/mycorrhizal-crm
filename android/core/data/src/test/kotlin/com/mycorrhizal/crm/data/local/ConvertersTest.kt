package com.mycorrhizal.crm.data.local

import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.CRMEnvelope
import com.mycorrhizal.crm.model.network.Name
import kotlinx.datetime.Instant
import kotlinx.datetime.LocalDate
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class ConvertersTest {

    private val converters = Converters()

    @Test
    fun `string list round-trips through JSON`() {
        val values = listOf("a", "b", "c")

        val json = converters.fromStringList(values)
        val restored = converters.toStringList(json)

        assertEquals(values, restored)
    }

    @Test
    fun `null string list round-trips to null`() {
        assertNull(converters.fromStringList(null))
        assertNull(converters.toStringList(null))
    }

    @Test
    fun `card round-trips through JSON`() {
        val card = Card(uid = "u1", kind = "individual", name = Name(full = "Alice Smith"))

        val json = converters.fromCard(card)
        val restored = converters.toCard(json)

        assertEquals(card, restored)
    }

    @Test
    fun `crm envelope round-trips through JSON`() {
        val envelope = CRMEnvelope(kind = "human", circles = listOf("friends"))

        val json = converters.fromCrmEnvelope(envelope)
        val restored = converters.toCrmEnvelope(json)

        assertEquals(envelope, restored)
    }

    @Test
    fun `instant round-trips through epoch millis`() {
        val instant = Instant.fromEpochMilliseconds(1_700_000_000_000)

        val millis = converters.fromInstant(instant)
        val restored = converters.toInstant(millis)

        assertEquals(instant, restored)
    }

    @Test
    fun `null instant round-trips to null`() {
        assertNull(converters.fromInstant(null))
        assertNull(converters.toInstant(null))
    }

    @Test
    fun `local date round-trips through ISO string`() {
        val date = LocalDate(2026, 8, 21)

        val text = converters.fromLocalDate(date)
        val restored = converters.toLocalDate(text)

        assertEquals("2026-08-21", text)
        assertEquals(date, restored)
    }

    @Test
    fun `null local date round-trips to null`() {
        assertNull(converters.fromLocalDate(null))
        assertNull(converters.toLocalDate(null))
    }
}
