package com.mycorrhizal.crm.network

import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactEntityLinkFromExistingInput
import com.mycorrhizal.crm.model.network.ContactEntityLinkInput
import com.mycorrhizal.crm.model.network.ContactEntityLinkSource
import com.mycorrhizal.crm.model.network.ContactFieldValuesInput
import com.mycorrhizal.crm.model.network.ContactRecordInput
import com.mycorrhizal.crm.model.network.EntityInput
import com.mycorrhizal.crm.model.network.EntityKind
import com.mycorrhizal.crm.model.network.EntityMergeInput
import com.mycorrhizal.crm.model.network.FieldValueInput
import com.mycorrhizal.crm.model.network.Organization
import com.squareup.moshi.Moshi
import com.squareup.moshi.kotlin.reflect.KotlinJsonAdapterFactory
import kotlinx.coroutines.runBlocking
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/** Issue #2302: the entity and contact-entity-link endpoints (ADR 0043 decision 10). */
class ApiClientEntitiesTest {

    private lateinit var server: MockWebServer
    private lateinit var client: ApiClient

    @Before
    fun setup() {
        server = MockWebServer()
        server.start()
        val okHttp = OkHttpClient.Builder()
            .addInterceptor(
                BaseUrlInterceptor(BaseUrlProvider { server.url("/").toString().trimEnd('/') })
            )
            .build()
        client = ApiClient(okHttp, Moshi.Builder().add(KotlinJsonAdapterFactory()).build())
    }

    @After
    fun teardown() {
        server.shutdown()
    }

    private fun enqueue(body: String, code: Int = 200) {
        server.enqueue(MockResponse().setResponseCode(code).setBody(body))
    }

    private val entityJson =
        """{"id":"e1","kind":"organisation","name":"Acme","core":{"url":"u"},"sensitivity":"normal","revision":3}"""

    private val linkJson = """{"id":"l1","contact_vcard_uid":"u1","entity_id":"e1","entity_name":"Acme",
        "kind":"organisation","role":"employee","role_label":"","is_primary":true,
        "period":{"start":{"year":2019}},"sensitivity":"normal","current":true,"projects_as":"ORG","revision":2}"""

    @Test
    fun `listEntities sends the kind, search and cursor and tolerates a null list`() = runBlocking {
        enqueue("""{"entities":null,"next_cursor":"","limit":50}""")
        val result = client.listEntities(kind = "place", q = "cab in", cursor = "c1", limit = 50)
        val request = server.takeRequest()
        assertEquals("GET", request.method)
        assertEquals("/api/v1/entities", request.requestUrl?.encodedPath)
        assertEquals("place", request.requestUrl?.queryParameter("kind"))
        assertEquals("cab in", request.requestUrl?.queryParameter("q"))
        assertEquals("c1", request.requestUrl?.queryParameter("cursor"))
        assertEquals("50", request.requestUrl?.queryParameter("limit"))
        assertTrue(result.getOrThrow().entitiesOrEmpty.isEmpty())
    }

    @Test
    fun `getEntity reads the counts and the core map`() = runBlocking {
        enqueue(
            """{"id":"e1","kind":"pet","name":"Rex","core":{"birth":{"year":2019}},"sensitivity":"private",
            "revision":4,"link_count":2,"projection_count":0}"""
        )
        val entity = client.getEntity("e1").getOrThrow()
        assertEquals("/api/v1/entities/e1", server.takeRequest().requestUrl?.encodedPath)
        assertEquals(2, entity.linkCount)
        assertEquals(4, entity.revision)
        assertEquals(mapOf("year" to 2019.0), entity.core?.get("birth"))
    }

    @Test
    fun `createEntity posts the input and returns the raw entity`() = runBlocking {
        enqueue(entityJson, 201)
        val created = client.createEntity(
            EntityInput(kind = EntityKind.ORGANISATION, name = "Acme", core = mapOf("url" to "u"))
        ).getOrThrow()
        val request = server.takeRequest()
        assertEquals("POST", request.method)
        assertTrue(request.body.readUtf8().contains(""""name":"Acme""""))
        assertEquals("e1", created.id)
    }

    @Test
    fun `updateEntity carries If-Match, deleteEntity returns the counts, restore posts empty`() =
        runBlocking {
            enqueue(entityJson)
            client.updateEntity("e1", EntityInput(name = "Acme"), 3).getOrThrow()
            val put = server.takeRequest()
            assertEquals("PUT", put.method)
            assertEquals("\"3\"", put.getHeader("If-Match"))

            enqueue("""{"message":"Entity deleted","link_count":5,"projection_count":2}""")
            val deleted = client.deleteEntity("e1", 3).getOrThrow()
            val del = server.takeRequest()
            assertEquals("DELETE", del.method)
            assertEquals("\"3\"", del.getHeader("If-Match"))
            assertEquals(5, deleted.linkCount)
            assertEquals(2, deleted.projectionCount)

            enqueue(
                """{"entity":$entityJson,"links_revived":4,"same_name_live":true,"skipped":null,
                "merged_into":"e2"}"""
            )
            val restored = client.restoreEntity("e1").getOrThrow()
            val post = server.takeRequest()
            assertEquals("POST", post.method)
            assertEquals("/api/v1/entities/e1/restore", post.requestUrl?.encodedPath)
            assertTrue(restored.sameNameLive)
            assertEquals("e2", restored.mergedInto)
        }

    @Test
    fun `mergeEntity posts into and the resolutions`() = runBlocking {
        enqueue(
            """{"survivor":$entityJson,"loser_id":"e9","links_repointed":3,"links_collapsed":0,
            "primaries_demoted":null,"field_values_moved":0,"field_values_dropped":null,
            "resolved_fields":["notes"],"restore_scope":"entity_only"}"""
        )
        val result = client.mergeEntity(
            "e9",
            EntityMergeInput(into = "e1", resolutions = mapOf("notes" to "loser"))
        ).getOrThrow()
        val request = server.takeRequest()
        assertEquals("/api/v1/entities/e9/merge", request.requestUrl?.encodedPath)
        val body = request.body.readUtf8()
        assertTrue(body.contains(""""into":"e1""""))
        assertTrue(body.contains(""""notes":"loser""""))
        assertEquals(3, result.linksRepointed)
        assertEquals(listOf("notes"), result.resolvedFields)
    }

    @Test
    fun `entity contacts and field values unwrap their envelopes`() = runBlocking {
        enqueue(
            """{"contacts":[{"link_id":"l1","contact_id":7,"contact_vcard_uid":"u","firstname":"Ada",
            "lastname":"L","role":"employee","role_label":"","is_primary":true,"sensitivity":"normal"}]}"""
        )
        val people = client.listEntityContacts("e1").getOrThrow()
        assertEquals("/api/v1/entities/e1/contacts", server.takeRequest().requestUrl?.encodedPath)
        assertEquals("Ada L", people.single().displayName)

        enqueue("""{"field_values":null}""")
        assertTrue(client.listEntityFieldValues("e1").getOrThrow().fieldValues.isNullOrEmpty())
        assertEquals(
            "/api/v1/entities/e1/field-values",
            server.takeRequest().requestUrl?.encodedPath
        )

        enqueue("""{"message":"ok","field_values":[]}""")
        client.replaceEntityFieldValues(
            "e1",
            ContactFieldValuesInput(listOf(FieldValueInput("d1", "x")))
        ).getOrThrow()
        val put = server.takeRequest()
        assertEquals("PUT", put.method)
        assertEquals("/api/v1/entities/e1/field-values", put.requestUrl?.encodedPath)
    }

    @Test
    fun `contact entity link CRUD hits the nested routes`() = runBlocking {
        enqueue("""{"links":[$linkJson]}""")
        val links = client.listContactEntityLinks(5).getOrThrow()
        assertEquals(
            "/api/v1/contacts/5/entity-links",
            server.takeRequest().requestUrl?.encodedPath
        )
        assertEquals("ORG", links.single().projectsAs)

        enqueue(linkJson, 201)
        client.createContactEntityLink(
            5,
            ContactEntityLinkInput(entityId = "e1", role = "employee", isPrimary = true)
        ).getOrThrow()
        val post = server.takeRequest()
        assertEquals("POST", post.method)
        assertTrue(post.body.readUtf8().contains(""""entity_id":"e1""""))

        enqueue(linkJson)
        client.updateContactEntityLink(
            5,
            "l1",
            ContactEntityLinkInput(role = "owner"),
            2
        ).getOrThrow()
        val put = server.takeRequest()
        assertEquals("PUT", put.method)
        assertEquals("/api/v1/contacts/5/entity-links/l1", put.requestUrl?.encodedPath)
        assertEquals("\"2\"", put.getHeader("If-Match"))

        enqueue("""{"message":"Entity link deleted"}""")
        assertTrue(client.deleteContactEntityLink(5, "l1").isSuccess)
        assertEquals("DELETE", server.takeRequest().method)
    }

    @Test
    fun `adopt and link-from-existing hit their own routes`() = runBlocking {
        enqueue(
            """{"link":$linkJson,"adopted":true,"element_kind":"organization","element_id":"o9"}"""
        )
        val adopted = client.adoptContactEntityLink(5, "l1").getOrThrow()
        val adopt = server.takeRequest()
        assertEquals("POST", adopt.method)
        assertEquals("/api/v1/contacts/5/entity-links/l1/adopt", adopt.requestUrl?.encodedPath)
        assertTrue(adopted.adopted)
        assertEquals("o9", adopted.elementId)

        enqueue("""{"link":$linkJson,"entity":$entityJson,"entity_created":true}""", 201)
        val linked = client.linkContactEntityFromExisting(
            5,
            ContactEntityLinkFromExistingInput(
                source = ContactEntityLinkSource(kind = "organization", id = "o1"),
                role = "employee"
            )
        ).getOrThrow()
        val request = server.takeRequest()
        assertEquals(
            "/api/v1/contacts/5/entity-links/from-existing",
            request.requestUrl?.encodedPath
        )
        val body = request.body.readUtf8()
        assertTrue(body.contains(""""source":{"kind":"organization","id":"o1"}"""))
        assertFalse(body.contains("entity_id"))
        assertTrue(linked.entityCreated)
    }

    @Test
    fun `the graph overlay requests include_entities only when asked and reads the entity keys`() =
        runBlocking {
            enqueue(
                """{"nodes":[{"id":"c-7","type":"contact","label":"Ada"}],
                "entity_nodes":[{"id":"e-e1","kind":"organisation","label":"Acme"}],
                "entity_edges":[{"id":"x","source":"c-7","target":"e-e1"}]}"""
            )
            val response = client.getGraphWithEntities(from = "u1", depth = 2).getOrThrow()
            val request = server.takeRequest()
            assertEquals("/api/v1/graph", request.requestUrl?.encodedPath)
            assertEquals("true", request.requestUrl?.queryParameter("include_entities"))
            assertEquals("u1", request.requestUrl?.queryParameter("from"))
            assertEquals("2", request.requestUrl?.queryParameter("depth"))
            assertEquals("Acme", response.entityNodes?.single()?.label)
            assertEquals("c-7", response.entityEdges?.single()?.source)
            assertNull(response.nodes?.firstOrNull { it.type == "activity" })
        }

    @Test
    fun `updateContact never sends a projected ent- entry back`() = runBlocking {
        enqueue("""{"id":1,"revision":2}""")
        client.updateContact(
            1,
            ContactRecordInput(
                card = Card(
                    organizations = listOf(
                        Organization(id = "o1", name = "Own"),
                        Organization(id = "ent-l1", name = "Projected")
                    )
                )
            ),
            1
        ).getOrThrow()
        val body = server.takeRequest().body.readUtf8()
        assertTrue(body.contains("Own"))
        assertFalse(body.contains("Projected"))
        assertFalse(body.contains("ent-l1"))
    }
}
