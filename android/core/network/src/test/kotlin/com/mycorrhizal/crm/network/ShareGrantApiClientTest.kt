package com.mycorrhizal.crm.network

import com.mycorrhizal.crm.model.network.ShareGrantInput
import com.mycorrhizal.crm.model.network.ShareGrantPatch
import com.squareup.moshi.Moshi
import com.squareup.moshi.kotlin.reflect.KotlinJsonAdapterFactory
import kotlinx.coroutines.runBlocking
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/** ADR 0042 decision 12: the standing-share client against the real route shapes. */
class ShareGrantApiClientTest {

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

    private val outgoingJson = """{"id":"g1","to_user_id":9,"to_username":"casey",
        "entity_type":"circle","entity_id":"7","label":"Family","sections":[],
        "reconfirm_sections":["addresses"],"status":"pending","expires_at":null,
        "responded_at":null,"revoked_at":null,"revoked_reason":null,"member_count":3,
        "created_at":"2026-11-01T00:00:00Z","updated_at":"2026-11-01T00:00:00Z","revision":4}"""

    @Test
    fun `incoming list parses the recipient view and sends the cursor`() = runBlocking {
        server.enqueue(
            MockResponse().setBody(
                """{"share_grants":[{"id":"g1","from_username":"dana","entity_type":"contact",
                "label":"Dana White","sections":["emails"],"status":"pending","expires_at":null,
                "responded_at":null,"created_at":"2026-11-01T00:00:00Z"}],
                "next_cursor":"nx","limit":50}"""
            )
        )
        val page = client.listIncomingShareGrants(cursor = "c1", limit = 50).getOrThrow()
        val request = server.takeRequest()
        assertEquals("GET", request.method)
        assertEquals("/api/v1/share-grants/incoming?cursor=c1&limit=50", request.path)
        assertEquals("dana", page.shareGrants.single().fromUsername)
        assertEquals("Dana White", page.shareGrants.single().label)
        assertEquals("nx", page.nextCursor)
    }

    @Test
    fun `outgoing list parses the owner view`() = runBlocking {
        server.enqueue(MockResponse().setBody("""{"share_grants":[$outgoingJson],"limit":50}"""))
        val grant = client.listOutgoingShareGrants().getOrThrow().shareGrants.single()
        assertEquals("/api/v1/share-grants/outgoing", server.takeRequest().path)
        assertEquals("casey", grant.toUsername)
        assertEquals(listOf("addresses"), grant.reconfirmSections)
        assertEquals(3, grant.memberCount)
        assertEquals(4, grant.revision)
    }

    @Test
    fun `create posts only the allowlisted body and unwraps share_grant`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(201).setBody("""{"share_grant":$outgoingJson}""")
        )
        val grant = client.createShareGrant(
            ShareGrantInput(
                toUserId = 9,
                entityType = "contact",
                entityId = "uid-1",
                sections = listOf("emails"),
                expiresAt = "2026-12-01T00:00:00Z"
            )
        ).getOrThrow()
        val request = server.takeRequest()
        assertEquals("POST", request.method)
        assertEquals("/api/v1/share-grants", request.path)
        val body = request.body.readUtf8()
        assertTrue(body.contains("\"to_user_id\":9"))
        assertTrue(body.contains("\"entity_id\":\"uid-1\""))
        assertTrue(body.contains("\"sections\":[\"emails\"]"))
        assertTrue(!body.contains("sensitive"))
        assertEquals("g1", grant.id)
    }

    @Test
    fun `patch sends If-Match and an explicit null to clear the expiry`() = runBlocking {
        server.enqueue(MockResponse().setBody("""{"share_grant":$outgoingJson}"""))
        client.updateShareGrant(
            "g1",
            ShareGrantPatch(clearExpiry = true),
            revision = 4
        ).getOrThrow()
        val request = server.takeRequest()
        assertEquals("PATCH", request.method)
        assertEquals("/api/v1/share-grants/g1", request.path)
        assertEquals("\"4\"", request.getHeader("If-Match"))
        assertEquals("""{"expires_at":null}""", request.body.readUtf8())
    }

    @Test
    fun `patch narrows sections and sets the expiry`() = runBlocking {
        server.enqueue(MockResponse().setBody("""{"share_grant":$outgoingJson}"""))
        client.updateShareGrant(
            "g1",
            ShareGrantPatch(expiresAt = "2026-12-01T00:00:00Z", sections = listOf("emails")),
            revision = 4
        ).getOrThrow()
        assertEquals(
            """{"expires_at":"2026-12-01T00:00:00Z","sections":["emails"]}""",
            server.takeRequest().body.readUtf8()
        )
    }

    @Test
    fun `patch with nothing to change omits every field`() = runBlocking {
        server.enqueue(MockResponse().setBody("""{"share_grant":$outgoingJson}"""))
        client.updateShareGrant("g1", ShareGrantPatch(), revision = null).getOrThrow()
        val request = server.takeRequest()
        assertEquals("{}", request.body.readUtf8())
        assertNull(request.getHeader("If-Match"))
    }

    @Test
    fun `recipient actions post to their routes`() = runBlocking {
        val incoming = """{"share_grant":{"id":"g1","from_username":"dana","entity_type":"note",
            "label":"Note","sections":[],"status":"active"}}"""
        repeat(2) { server.enqueue(MockResponse().setBody(incoming)) }
        assertEquals("active", client.acceptShareGrant("g1").getOrThrow().status)
        client.leaveShareGrant("g1").getOrThrow()
        assertEquals("/api/v1/share-grants/g1/accept", server.takeRequest().path)
        assertEquals("/api/v1/share-grants/g1/leave", server.takeRequest().path)
    }

    @Test
    fun `decline sends block only when asked`() = runBlocking {
        val incoming = """{"share_grant":{"id":"g1","from_username":"dana","entity_type":"note",
            "label":"Note","sections":[],"status":"declined"}}"""
        repeat(2) { server.enqueue(MockResponse().setBody(incoming)) }
        client.declineShareGrant("g1", block = false).getOrThrow()
        client.declineShareGrant("g1", block = true).getOrThrow()
        val first = server.takeRequest()
        val second = server.takeRequest()
        assertEquals("/api/v1/share-grants/g1/decline", first.path)
        assertTrue(!first.body.readUtf8().contains("\"block\":true"))
        assertEquals("""{"block":true}""", second.body.readUtf8())
    }

    @Test
    fun `owner actions post to revoke and reconfirm`() = runBlocking {
        repeat(2) { server.enqueue(MockResponse().setBody("""{"share_grant":$outgoingJson}""")) }
        client.revokeShareGrant("g1").getOrThrow()
        client.reconfirmShareGrant("g1").getOrThrow()
        assertEquals("/api/v1/share-grants/g1/revoke", server.takeRequest().path)
        assertEquals("/api/v1/share-grants/g1/reconfirm", server.takeRequest().path)
    }

    @Test
    fun `content parses each allowlisted view and keeps the contact as a generic map`() =
        runBlocking {
            server.enqueue(
                MockResponse().setBody(
                    """{"share_grant_content":{"grant_id":"g1","entity_type":"contact",
                "owner_username":"dana","contact":{"@type":"Card","name":{"full":"Dana White"}}}}"""
                )
            )
            val content = client.getShareGrantContent("g1").getOrThrow()
            val request = server.takeRequest()
            assertEquals("GET", request.method)
            assertEquals("/api/v1/share-grants/g1/content", request.path)
            assertEquals("dana", content.ownerUsername)
            assertEquals("Card", content.contact?.get("@type"))
        }

    @Test
    fun `content parses circle members`() = runBlocking {
        server.enqueue(
            MockResponse().setBody(
                """{"share_grant_content":{"grant_id":"g1","entity_type":"circle",
                "owner_username":"dana","circle":{"name":"Family","description":"d","color":"#fff",
                "member_count":2,"members":[{"display_name":"A B"}],"members_truncated":true}}}"""
            )
        )
        val circle = client.getShareGrantContent("g1").getOrThrow().circle!!
        assertEquals("Family", circle.name)
        assertEquals("A B", circle.members.single().displayName)
        assertTrue(circle.membersTruncated)
    }

    @Test
    fun `a revoked grant surfaces the identical 404 as an ApiError`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(404).setBody(
                """{"error":{"code":"NOT_FOUND","message":"Share grant not found",
                "reason_code":"share_grant_not_found"}}"""
            )
        )
        val error = client.getShareGrantContent("g1").exceptionOrNull() as ApiError.Client
        assertEquals(404, error.code)
        assertEquals("share_grant_not_found", error.reason?.code)
    }

    @Test
    fun `blocks list and unblock`() = runBlocking {
        server.enqueue(MockResponse().setBody("""{"blocks":[{"user_id":7,"username":"dana"}]}"""))
        server.enqueue(MockResponse().setResponseCode(204))
        assertEquals("dana", client.listShareBlocks().getOrThrow().single().username)
        client.deleteShareBlock(7).getOrThrow()
        assertEquals("/api/v1/share-grants/blocks", server.takeRequest().path)
        val del = server.takeRequest()
        assertEquals("DELETE", del.method)
        assertEquals("/api/v1/share-grants/blocks/7", del.path)
    }

    @Test
    fun `notifications expose the pending badge count and can be marked read`() = runBlocking {
        server.enqueue(
            MockResponse().setBody(
                """{"pending_count":2,"notifications":[{"id":1,"kind":"pending",
                "entity_type":"contact","sender_username":"dana","created_at":"2026-11-01T00:00:00Z"}]}"""
            )
        )
        server.enqueue(MockResponse().setBody("""{"marked":1}"""))
        val n = client.getShareGrantNotifications().getOrThrow()
        assertEquals(2, n.pendingCount)
        assertEquals("dana", n.notifications.single().senderUsername)
        client.markShareGrantNotificationsRead().getOrThrow()
        assertEquals("/api/v1/share-grants/notifications", server.takeRequest().path)
        val mark = server.takeRequest()
        assertEquals("POST", mark.method)
        assertEquals("/api/v1/share-grants/notifications/read", mark.path)
    }
}
