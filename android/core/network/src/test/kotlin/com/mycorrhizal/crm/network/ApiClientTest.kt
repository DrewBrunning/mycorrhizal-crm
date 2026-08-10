package com.mycorrhizal.crm.network

import com.mycorrhizal.crm.model.network.LoginResponse
import com.squareup.moshi.Moshi
import com.squareup.moshi.kotlin.reflect.KotlinJsonAdapterFactory
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import kotlinx.coroutines.runBlocking

class ApiClientTest {

    private lateinit var server: MockWebServer
    private lateinit var client: ApiClient

    @Before
    fun setup() {
        server = MockWebServer()
        server.start()
        val okHttp = OkHttpClient.Builder()
            .addInterceptor(
                BaseUrlInterceptor(BaseUrlProvider { server.url("/").toString().trimEnd('/') }),
            )
            .build()
        val moshi = Moshi.Builder()
            .add(KotlinJsonAdapterFactory())
            .build()
        client = ApiClient(okHttp, moshi)
    }

    @After
    fun teardown() {
        server.shutdown()
    }

    @Test
    fun `login captures the auth_token cookie and parses preferences`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setHeader("Set-Cookie", "auth_token=eyJhbGciOiJIUzI1NiJ9.abc; Path=/; HttpOnly")
                .setBody("""{"language":"en","date_format":"eu"}"""),
        )

        val result = client.login("alice", "secret")

        assertTrue(result.isSuccess)
        val login = result.getOrThrow()
        assertEquals("eyJhbGciOiJIUzI1NiJ9.abc", login.token)
        assertEquals("en", login.language)
        assertEquals("eu", login.dateFormat)

        val request = server.takeRequest()
        assertEquals("POST", request.method)
        assertEquals("/api/v1/login", request.path)
    }

    @Test
    fun `login returns null token when no auth cookie is set`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody("""{"language":"en","date_format":"eu"}"""),
        )

        val result = client.login("alice", "secret")

        assertTrue(result.isSuccess)
        assertEquals(null, result.getOrThrow().token)
    }

    @Test
    fun `login with bad credentials maps to Client 401`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(401)
                .setBody("""{"error":{"code":"invalid_credentials","message":"Invalid credentials"}}"""),
        )

        val result = client.login("alice", "wrong")

        assertTrue(result.isFailure)
        val error = result.exceptionOrNull() as ApiError
        assertTrue(error is ApiError.Client)
        assertEquals(401, (error as ApiError.Client).code)
    }

    @Test
    fun `list contacts parses the cursor page`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody(
                    """
                    {
                      "contacts": [
                        {"id": 1, "uid": "u1", "fn": "Alice Smith", "firstname": "Alice", "lastname": "Smith", "primary_email": "a@x.com"},
                        {"id": 2, "uid": "u2", "fn": "Bob Jones"}
                      ],
                      "next_cursor": "cursor-2",
                      "limit": 50,
                      "sync": {"mode": "incremental", "incremental": ["contacts"]}
                    }
                    """.trimIndent(),
                ),
        )

        val result = client.listContacts()

        assertTrue(result.isSuccess)
        val page = result.getOrThrow()
        assertEquals(2, page.contacts.size)
        assertEquals("Alice Smith", page.contacts[0].fn)
        assertEquals("cursor-2", page.nextCursor)
        assertEquals("incremental", page.sync?.mode)
    }

    @Test
    fun `list contacts sends query parameters`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody("""{"contacts":[],"next_cursor":""}"""),
        )

        client.listContacts(cursor = "c1", limit = 25, search = "ali")

        val request = server.takeRequest()
        assertEquals("/api/v1/contacts?cursor=c1&limit=25&search=ali", request.path)
    }

    @Test
    fun `get contact parses the full record`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody(
                    """
                    {
                      "id": 7,
                      "uid": "u7",
                      "etag": "abc",
                      "card": {
                        "name": {"full": "Carol King", "components": [{"kind": "given", "value": "Carol"}]},
                        "emails": [{"address": "carol@x.com", "contexts": ["work"]}],
                        "phones": [{"number": "+1-555-0100", "features": ["cell"]}]
                      },
                      "crm": {"circles": ["friends"]},
                      "archived": false
                    }
                    """.trimIndent(),
                ),
        )

        val result = client.getContact(7)

        assertTrue(result.isSuccess)
        val record = result.getOrThrow()
        assertEquals(7, record.id)
        assertEquals("Carol King", record.card?.name?.full)
        assertEquals("carol@x.com", record.card?.emails?.firstOrNull()?.address)
        assertEquals(listOf("friends"), record.crm?.circles)

        val request = server.takeRequest()
        assertEquals("/api/v1/contacts/7", request.path)
    }

    @Test
    fun `current user parses profile`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody(
                    """{"id": 3, "username": "alice", "email": "a@x.com", "is_admin": true, "language": "de"}""",
                ),
        )

        val result = client.currentUser()

        assertTrue(result.isSuccess)
        val profile = result.getOrThrow()
        assertEquals(3, profile.id)
        assertEquals("alice", profile.username)
        assertTrue(profile.isAdmin)
        assertEquals("de", profile.language)
    }

    @Test
    fun `404 maps to Client error`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(404)
                .setBody("""{"error":{"code":"not_found","message":"Contact not found"}}"""),
        )

        val result = client.getContact(999)

        assertTrue(result.isFailure)
        val error = result.exceptionOrNull() as ApiError
        assertTrue(error is ApiError.Client)
        assertEquals(404, (error as ApiError.Client).code)
        assertEquals("Contact not found", error.body)
    }

    @Test
    fun `malformed json maps to Parse error`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody("""{"contacts": [}"""),
        )

        val result = client.listContacts()

        assertTrue(result.isFailure)
        val error = result.exceptionOrNull()
        assertNotNull(error)
    }
}
