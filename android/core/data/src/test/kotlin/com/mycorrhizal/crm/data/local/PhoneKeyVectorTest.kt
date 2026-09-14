package com.mycorrhizal.crm.data.local

import org.junit.Assert.assertEquals
import org.junit.Assert.fail
import org.junit.Test

/**
 * Issue #963: the Android [PhoneKey] is a deliberate line-for-line port of the
 * backend `models.PhoneKey`/`NormalizePhoneDigits`. This test reads the SAME
 * shared vector table (`/testdata/phonekey-vectors/vectors.json`, wired onto
 * this module's test classpath by build.gradle.kts) that
 * `backend/models/phonekey_vectors_test.go` reads, so a change to one port's
 * normalization that the other doesn't follow fails here rather than shipping
 * as an on-device matching bug.
 */
class PhoneKeyVectorTest {

    private data class Vector(val input: String, val digits: String, val key: String, val why: String)

    private fun loadTable(): Pair<List<Vector>, List<Pair<String, List<String>>>> {
        val stream = checkNotNull(javaClass.classLoader?.getResourceAsStream("vectors.json")) {
            "missing shared phonekey vectors -- check core/data build.gradle.kts resources.srcDir"
        }
        val root = com.mycorrhizal.crm.model.MoshiProvider.get()
            .adapter(Any::class.java)
            .fromJson(stream.bufferedReader().readText()) as Map<*, *>

        val vectors = (root["vectors"] as List<*>).map { row ->
            val m = row as Map<*, *>
            Vector(
                input = m["input"] as String,
                digits = m["digits"] as String,
                key = m["key"] as String,
                why = m["why"] as String,
            )
        }
        val groups = (root["same_key_groups"] as List<*>).map { row ->
            val m = row as Map<*, *>
            (m["why"] as String) to (m["inputs"] as List<*>).map { it as String }
        }
        return vectors to groups
    }

    @Test
    fun `every shared vector matches normalizeDigits and key`() {
        val (vectors, _) = loadTable()
        check(vectors.isNotEmpty()) { "shared phonekey vectors are empty" }

        for (v in vectors) {
            assertEquals("normalizeDigits(${v.input}) [${v.why}]", v.digits, PhoneKey.normalizeDigits(v.input))
            assertEquals("key(${v.input}) [${v.why}]", v.key, PhoneKey.key(v.input))
        }
    }

    @Test
    fun `every same-key group collapses to one key`() {
        val (_, groups) = loadTable()
        check(groups.isNotEmpty()) { "shared same_key_groups are empty" }

        for ((why, inputs) in groups) {
            if (inputs.size < 2) {
                fail("same_key_groups entry '$why' needs at least two inputs")
            }
            val want = PhoneKey.key(inputs.first())
            check(want.isNotEmpty()) { "same_key_groups entry '$why' first input keys to empty" }
            for (input in inputs.drop(1)) {
                assertEquals("key($input) should equal key(${inputs.first()}) [$why]", want, PhoneKey.key(input))
            }
        }
    }
}
