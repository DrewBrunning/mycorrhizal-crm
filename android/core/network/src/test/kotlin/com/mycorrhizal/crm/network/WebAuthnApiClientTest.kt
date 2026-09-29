package com.mycorrhizal.crm.network

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

/** Issue #1293: `methods` parsing on POST /login and the webauthn API calls (backend #593, #1317). */
class WebAuthnApiClientTest {

    private lateinit var server: MockWebServer
    private lateinit var client: ApiClient

    @Before
    fun setup() {
        server = MockWebServer()
        server.start()
        val okHttp = OkHttpClient.Builder()
            .addInterceptor(BaseUrlInterceptor(BaseUrlProvider { server.url("/").toString().trimEnd('/') }))
            .build()
        client = ApiClient(okHttp, Moshi.Builder().add(KotlinJsonAdapterFactory()).build())
    }

    @After
    fun teardown() {
        server.shutdown()
    }

    private fun twoFactorLogin(body: String) {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setHeader("Set-Cookie", "2fa_pending=chal; Path=/; HttpOnly")
                .setBody(body),
        )
    }

    private fun clientError(result: Result<*>, code: Int): ApiError.Client {
        val error = result.exceptionOrNull() as ApiError
        assertTrue("expected Client error, was $error", error is ApiError.Client)
        assertEquals(code, (error as ApiError.Client).code)
        return error
    }

    // --- `methods` on POST /login ---

    @Test
    fun `login parses methods for a mixed account`() = runBlocking {
        twoFactorLogin("""{"two_factor_required":true,"methods":["totp","webauthn"]}""")
        assertEquals(listOf("totp", "webauthn"), client.login("a", "b").getOrThrow().methods)
    }

    @Test
    fun `login parses methods for a passkey-only account`() = runBlocking {
        twoFactorLogin("""{"two_factor_required":true,"methods":["webauthn"]}""")
        assertEquals(listOf("webauthn"), client.login("a", "b").getOrThrow().methods)
    }

    @Test
    fun `login parses a totp-only methods list`() = runBlocking {
        twoFactorLogin("""{"two_factor_required":true,"methods":["totp"]}""")
        assertEquals(listOf("totp"), client.login("a", "b").getOrThrow().methods)
    }

    @Test
    fun `login keeps unknown method tokens verbatim`() = runBlocking {
        twoFactorLogin("""{"two_factor_required":true,"methods":["totp","hwkey"]}""")
        assertEquals(listOf("totp", "hwkey"), client.login("a", "b").getOrThrow().methods)
    }

    @Test
    fun `login from an older server without methods reports null`() = runBlocking {
        twoFactorLogin("""{"two_factor_required":true}""")
        val login = client.login("a", "b").getOrThrow()
        assertTrue(login.twoFactorRequired)
        assertNull(login.methods)
    }

    @Test
    fun `methods are ignored when two factor is not required`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(200)
                .setHeader("Set-Cookie", "auth_token=jwt; Path=/")
                .setBody("""{"language":"en","methods":["webauthn"]}"""),
        )
        assertNull(client.login("a", "b").getOrThrow().methods)
    }

    // --- login ceremony ---

    @Test
    fun `webauthnLoginBegin posts with the pending cookie and returns the raw options`() = runBlocking {
        val options = """{"publicKey":{"challenge":"abc","rpId":"example.com"}}"""
        server.enqueue(MockResponse().setResponseCode(200).setBody(options))

        val result = client.webauthnLoginBegin("chal-1")

        assertEquals(options, result.getOrThrow())
        val request = server.takeRequest()
        assertEquals("POST", request.method)
        assertEquals("/api/v1/webauthn/login/begin", request.path)
        assertEquals("2fa_pending=chal-1", request.getHeader("Cookie"))
    }

    @Test
    fun `webauthnLoginBegin maps 400 401 and 409`() = runBlocking {
        for (code in listOf(400, 401, 409)) {
            server.enqueue(MockResponse().setResponseCode(code).setBody("""{"error":{"code":"x","message":"m$code"}}"""))
            val error = clientError(client.webauthnLoginBegin("c"), code)
            assertEquals("m$code", error.message)
        }
    }

    @Test
    fun `webauthnLoginFinish forwards the credential verbatim and captures the session`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(200)
                .setHeader("Set-Cookie", "auth_token=pk-session; Path=/; HttpOnly")
                .setBody("""{"language":"de","date_format":"eu"}"""),
        )
        val credential = """{"id":"AAA","response":{"clientDataJSON":"e30"},"type":"public-key"}"""

        val login = client.webauthnLoginFinish(credential, "chal-1").getOrThrow()

        assertEquals("pk-session", login.token)
        assertEquals("de", login.language)
        assertEquals("eu", login.dateFormat)
        val request = server.takeRequest()
        assertEquals("POST", request.method)
        assertEquals("/api/v1/webauthn/login/finish", request.path)
        assertEquals("2fa_pending=chal-1", request.getHeader("Cookie"))
        assertEquals(credential, request.body.readUtf8())
    }

    @Test
    fun `webauthnLoginFinish maps rejection expiry client floor conflict and lockout`() = runBlocking {
        for (code in listOf(400, 401, 403, 409)) {
            server.enqueue(MockResponse().setResponseCode(code).setBody("""{"error":{"code":"x","message":"m$code"}}"""))
            clientError(client.webauthnLoginFinish("{}", "c"), code)
        }
        server.enqueue(
            MockResponse().setResponseCode(429).setBody("""{"error":"locked","message":"Try again later."}"""),
        )
        assertEquals("Try again later.", clientError(client.webauthnLoginFinish("{}", "c"), 429).message)
    }

    // --- enrollment ---

    @Test
    fun `webauthnRegisterBegin posts the optional name and returns raw options`() = runBlocking {
        val options = """{"publicKey":{"challenge":"c"}}"""
        server.enqueue(MockResponse().setResponseCode(200).setBody(options))

        assertEquals(options, client.webauthnRegisterBegin("Pixel 8a").getOrThrow())

        val request = server.takeRequest()
        assertEquals("POST", request.method)
        assertEquals("/api/v1/webauthn/register/begin", request.path)
        assertEquals("""{"name":"Pixel 8a"}""", request.body.readUtf8())
    }

    @Test
    fun `webauthnRegisterBegin omits a blank name`() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(200).setBody("{}"))
        client.webauthnRegisterBegin("   ").getOrThrow()
        assertEquals("{}", server.takeRequest().body.readUtf8())
    }

    @Test
    fun `webauthnRegisterBegin maps 400 403 and 409`() = runBlocking {
        for (code in listOf(400, 401, 403, 409)) {
            server.enqueue(MockResponse().setResponseCode(code).setBody("""{"error":{"code":"x","message":"m"}}"""))
            clientError(client.webauthnRegisterBegin(), code)
        }
    }

    @Test
    fun `webauthnRegisterFinish parses the 201 body and the reissued session`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(201)
                .setHeader("Set-Cookie", "auth_token=reissued; Path=/; HttpOnly")
                .setBody(
                    """{"id":"11111111-1111-1111-1111-111111111111","name":"Pixel","created_at":"2026-09-29T10:00:00Z","recovery_codes":["AAAAA-BBBBB-CCCCC"]}""",
                ),
        )
        val attestation = """{"id":"AAA","type":"public-key"}"""

        val result = client.webauthnRegisterFinish(attestation).getOrThrow()

        assertEquals("Pixel", result.value.name)
        assertEquals(listOf("AAAAA-BBBBB-CCCCC"), result.value.recoveryCodes)
        assertEquals("reissued", result.reissuedToken)
        val request = server.takeRequest()
        assertEquals("/api/v1/webauthn/register/finish", request.path)
        assertEquals(attestation, request.body.readUtf8())
    }

    @Test
    fun `webauthnRegisterFinish tolerates a missing cookie and empty recovery codes`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(201)
                .setBody("""{"id":"i","name":"n","created_at":"t","recovery_codes":[]}"""),
        )
        val result = client.webauthnRegisterFinish("{}").getOrThrow()
        assertNull(result.reissuedToken)
        assertTrue(result.value.recoveryCodes.isEmpty())
    }

    @Test
    fun `webauthnRegisterFinish maps 400 403 and 409`() = runBlocking {
        for (code in listOf(400, 401, 403, 409)) {
            server.enqueue(MockResponse().setResponseCode(code).setBody("""{"error":{"code":"x","message":"m"}}"""))
            clientError(client.webauthnRegisterFinish("{}"), code)
        }
    }

    // --- credential list + removal proof ---

    @Test
    fun `listWebAuthnCredentials parses credentials including a null last_used_at`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"credentials":[{"id":"a","name":"Phone","created_at":"2026-09-01T00:00:00Z","last_used_at":"2026-09-02T00:00:00Z"},{"id":"b","name":"Key","created_at":"2026-09-03T00:00:00Z","last_used_at":null}]}""",
            ),
        )

        val list = client.listWebAuthnCredentials().getOrThrow().credentials

        assertEquals(listOf("a", "b"), list.map { it.id })
        assertEquals("2026-09-02T00:00:00Z", list[0].lastUsedAt)
        assertNull(list[1].lastUsedAt)
        val request = server.takeRequest()
        assertEquals("GET", request.method)
        assertEquals("/api/v1/webauthn/credentials", request.path)
    }

    @Test
    fun `listWebAuthnCredentials maps 401`() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(401).setBody("""{"error":{"code":"x","message":"m"}}"""))
        clientError(client.listWebAuthnCredentials(), 401)
        Unit
    }

    @Test
    fun `webauthnProofBegin sends exclude_id and returns raw options`() = runBlocking {
        val options = """{"publicKey":{"challenge":"c"}}"""
        server.enqueue(MockResponse().setResponseCode(200).setBody(options))

        assertEquals(options, client.webauthnProofBegin("cred-1").getOrThrow())

        val request = server.takeRequest()
        assertEquals("POST", request.method)
        assertEquals("/api/v1/webauthn/assert/begin", request.path)
        assertEquals("""{"exclude_id":"cred-1"}""", request.body.readUtf8())
    }

    @Test
    fun `webauthnProofBegin without an exclude id sends an empty object`() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(200).setBody("{}"))
        client.webauthnProofBegin().getOrThrow()
        assertEquals("{}", server.takeRequest().body.readUtf8())
    }

    @Test
    fun `webauthnProofBegin maps 404 and 409`() = runBlocking {
        for (code in listOf(401, 404, 409)) {
            server.enqueue(MockResponse().setResponseCode(code).setBody("""{"error":{"code":"x","message":"m"}}"""))
            clientError(client.webauthnProofBegin("x"), code)
        }
    }

    @Test
    fun `deleteWebAuthnCredential sends a code proof`() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"message":"Passkey removed"}"""))

        val result = client.deleteWebAuthnCredential("cred-1", "123456").getOrThrow()

        assertEquals("Passkey removed", result.message)
        val request = server.takeRequest()
        assertEquals("DELETE", request.method)
        assertEquals("/api/v1/webauthn/credentials/cred-1", request.path)
        assertEquals("""{"code":"123456"}""", request.body.readUtf8())
    }

    @Test
    fun `deleteWebAuthnCredential percent-encodes the id as a single path segment`() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"message":"ok"}"""))
        client.deleteWebAuthnCredential("a/b", "1").getOrThrow()
        assertEquals("/api/v1/webauthn/credentials/a%2Fb", server.takeRequest().path)
    }

    @Test
    fun `deleteWebAuthnCredential maps 400 and 404`() = runBlocking {
        for (code in listOf(400, 401, 404)) {
            server.enqueue(MockResponse().setResponseCode(code).setBody("""{"error":{"code":"x","message":"m"}}"""))
            clientError(client.deleteWebAuthnCredential("id", "c"), code)
        }
    }

    @Test
    fun `deleteWebAuthnCredentialWithAssertion embeds the assertion object verbatim`() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"message":"Passkey removed"}"""))
        val assertion = """{"id":"AAA","response":{"signature":"sig"},"n":1}"""

        client.deleteWebAuthnCredentialWithAssertion("cred-1", assertion).getOrThrow()

        val request = server.takeRequest()
        assertEquals("DELETE", request.method)
        assertEquals("/api/v1/webauthn/credentials/cred-1", request.path)
        assertEquals("""{"assertion":$assertion}""", request.body.readUtf8())
    }

    @Test
    fun `deleteWebAuthnCredentialWithAssertion rejects a non-object assertion without a request`() = runBlocking {
        for (bad in listOf("not json", "[1]", "\"s\"")) {
            val error = client.deleteWebAuthnCredentialWithAssertion("id", bad).exceptionOrNull()
            assertTrue("for $bad: $error", error is ApiError.Parse)
        }
        assertEquals(0, server.requestCount)
    }

    @Test
    fun `deleteWebAuthnCredentialWithAssertion maps a rejected proof to Client 400`() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(400).setBody("""{"error":{"code":"x","message":"bad proof"}}"""))
        assertEquals("bad proof", clientError(client.deleteWebAuthnCredentialWithAssertion("i", "{}"), 400).message)
    }

    // --- issue #1337: enrollment proof on register/begin ---

    @Test
    fun `webauthnRegisterBegin sends a code proof next to the name`() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"publicKey":{}}"""))

        client.webauthnRegisterBegin("Pixel 8a", code = "AAAAA-BBBBB-CCCCC").getOrThrow()

        val request = server.takeRequest()
        assertEquals("/api/v1/webauthn/register/begin", request.path)
        assertEquals("""{"name":"Pixel 8a","code":"AAAAA-BBBBB-CCCCC"}""", request.body.readUtf8())
    }

    @Test
    fun `webauthnRegisterBegin sends a proof with no name and escapes the code`() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(200).setBody("{}"))
        server.enqueue(MockResponse().setResponseCode(200).setBody("{}"))

        client.webauthnRegisterBegin("   ", code = "12\"34").getOrThrow()
        assertEquals("""{"code":"12\"34"}""", server.takeRequest().body.readUtf8())

        client.webauthnRegisterBegin(null, code = "1", assertionJson = """{"id":"a"}""").getOrThrow()
        assertEquals("""{"code":"1","assertion":{"id":"a"}}""", server.takeRequest().body.readUtf8())
    }

    @Test
    fun `webauthnRegisterBegin embeds an assertion proof verbatim`() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(200).setBody("{}"))
        val assertion = """{"id":"AAA","response":{"signature":"sig"},"n":1}"""

        client.webauthnRegisterBegin("Tablet", assertionJson = assertion).getOrThrow()

        assertEquals("""{"name":"Tablet","assertion":$assertion}""", server.takeRequest().body.readUtf8())
    }

    @Test
    fun `webauthnRegisterBegin rejects a non-object assertion without a request`() = runBlocking {
        for (bad in listOf("not json", "[1]", "\"s\"")) {
            val error = client.webauthnRegisterBegin("x", assertionJson = bad).exceptionOrNull()
            assertTrue("for $bad: $error", error is ApiError.Parse)
        }
        assertEquals(0, server.requestCount)
    }

    @Test
    fun `webauthnRegisterBegin maps a rejected proof 400 and a lockout 429`() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(400).setBody("""{"error":{"code":"x","message":"Invalid code"}}"""))
        assertEquals("Invalid code", clientError(client.webauthnRegisterBegin("x", code = "0"), 400).message)
        server.enqueue(MockResponse().setResponseCode(429).setBody("""{"error":"locked","message":"Try again later."}"""))
        assertEquals("Try again later.", clientError(client.webauthnRegisterBegin("x", code = "0"), 429).message)
    }
}
