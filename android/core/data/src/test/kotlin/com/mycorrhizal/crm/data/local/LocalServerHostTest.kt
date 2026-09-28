package com.mycorrhizal.crm.data.local

import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.File

/**
 * Issue #1262: the embedded host's lifecycle and its secret boundary. The
 * launcher/health probe are fakes, so the start/stop/retry logic is exercised
 * without a real process or socket.
 */
class LocalServerHostTest {

    @get:Rule
    val temp = TemporaryFolder()

    private class FakeProcess(private val token: String?, val aliveAlways: Boolean = true) : LocalServerProcess {
        var destroyed = false

        override fun isAlive(): Boolean = aliveAlways && !destroyed

        override fun destroy() {
            destroyed = true
        }

        override fun awaitSessionToken(timeoutMs: Long): String? = token
    }

    private class RecordingLauncher(private val process: FakeProcess) : LocalServerProcessLauncher {
        var commands = mutableListOf<LocalServerCommand>()
        var calls = 0

        override fun launch(command: LocalServerCommand): LocalServerProcess {
            calls++
            commands += command
            return process
        }
    }

    private class FixedProbe(var healthy: Boolean) : LocalServerHealthProbe {
        override suspend fun isHealthy(socketPath: String): Boolean = healthy
    }

    private class IdentityCipher : LocalServerSecretCipher {
        override fun wrap(plaintext: ByteArray): ByteArray = plaintext
        override fun unwrap(blob: ByteArray): ByteArray = blob
    }

    private fun secretStore(root: File): LocalServerSecretStore = LocalServerSecretStore(
        keyFile = File(root, "local-server/keys.bin"),
        ciphers = LocalServerSecretCipherProvider { IdentityCipher() },
    )

    private fun host(
        launcher: RecordingLauncher,
        probe: FixedProbe = FixedProbe(true),
        binary: File? = null,
        root: File = temp.root,
    ): AndroidLocalServerHost {
        val binaryFile = binary ?: File(root, "libmycorrhizal.so").apply { writeText("fake") }
        return AndroidLocalServerHost(
            paths = LocalServerPaths.under(root),
            binaryProvider = LocalServerBinaryProvider { binaryFile.takeIf { it.exists() } },
            secretStore = secretStore(root),
            launcher = launcher,
            healthProbe = probe,
        )
    }

    @Test
    fun `ensureStarted launches once, returns the token, and never puts secrets in the environment`() = runBlocking {
        val process = FakeProcess(token = "local-jwt")
        val launcher = RecordingLauncher(process)
        val server = host(launcher)

        val result = server.ensureStarted()

        assertTrue("expected success, got $result", result.isSuccess)
        val endpoint = result.getOrThrow()
        assertEquals("local-jwt", endpoint.sessionToken)
        assertTrue(endpoint.socketPath.endsWith("local-server/sock"))
        assertEquals(1, launcher.calls)

        val command = launcher.commands.single()
        // The secrets travel on stdin only.
        assertTrue(command.stdinPayload.contains("\"jwt_secret\""))
        assertTrue(command.stdinPayload.contains("\"data_encryption_key\""))
        // The environment carries no secret.
        assertEquals(setOf("HOME", "TMPDIR"), command.environment.keys)
        assertTrue(command.environment.values.none { it.contains("jwt_secret") })
        assertEquals(endpoint.socketPath, server.socketPathIfRunning())
        assertEquals("local-jwt", server.sessionTokenIfRunning())

        // Idempotent: a second caller joins the running server.
        val again = server.ensureStarted()
        assertEquals(endpoint, again.getOrThrow())
        assertEquals(1, launcher.calls)
    }

    @Test
    fun `ensureStarted fails cleanly when the binary is absent`() = runBlocking {
        val launcher = RecordingLauncher(FakeProcess(token = "x"))
        val server = host(launcher, binary = File(temp.root, "does-not-exist"))

        val result = server.ensureStarted()

        assertTrue(result.isFailure)
        assertEquals(0, launcher.calls)
        assertNull(server.socketPathIfRunning())
    }

    @Test
    fun `ensureStarted destroys the process when readiness never arrives`() = runBlocking {
        val process = FakeProcess(token = null)
        val launcher = RecordingLauncher(process)
        val server = host(launcher)

        val result = server.ensureStarted()

        assertTrue(result.isFailure)
        assertTrue(process.destroyed)
        assertNull(server.socketPathIfRunning())
    }

    @Test
    fun `ensureStarted destroys the process when health never answers`() = runBlocking {
        val process = FakeProcess(token = "tok")
        val launcher = RecordingLauncher(process)
        val probe = FixedProbe(healthy = false)
        val server = host(launcher, probe = probe, root = temp.newFolder("healthfail"))

        val result = server.ensureStarted()

        assertTrue(result.isFailure)
        assertTrue(process.destroyed)
        assertNull(server.socketPathIfRunning())
    }

    @Test
    fun `a failed start can be retried`() = runBlocking {
        val dead = FakeProcess(token = null)
        val good = FakeProcess(token = "tok")
        val launcher = object : LocalServerProcessLauncher {
            var calls = 0
            override fun launch(command: LocalServerCommand): LocalServerProcess =
                if (calls++ == 0) dead else good
        }
        val root = temp.newFolder("retry")
        val server = AndroidLocalServerHost(
            paths = LocalServerPaths.under(root),
            binaryProvider = LocalServerBinaryProvider { File(root, "lib.so").apply { writeText("x") } },
            secretStore = secretStore(root),
            launcher = launcher,
            healthProbe = FixedProbe(true),
        )

        assertTrue(server.ensureStarted().isFailure)
        val retry = server.ensureStarted()
        assertTrue("retry must succeed, got $retry", retry.isSuccess)
        assertNotNull(server.socketPathIfRunning())
    }

    @Test
    fun `stop destroys the process and clears the endpoint`() = runBlocking {
        val process = FakeProcess(token = "tok")
        val server = host(RecordingLauncher(process))
        server.ensureStarted()

        server.stop()

        assertTrue(process.destroyed)
        assertNull(server.socketPathIfRunning())
        assertNull(server.sessionTokenIfRunning())
    }

    @Test
    fun `deleteLocalData stops the server and removes data and keys`() = runBlocking {
        val process = FakeProcess(token = "tok")
        val root = temp.newFolder("delete")
        val server = host(RecordingLauncher(process), root = root)
        server.ensureStarted()
        assertTrue(File(root, "local-server/keys.bin").exists())
        val dataDir = File(root, "local-server")
        assertTrue(dataDir.exists())

        server.deleteLocalData()

        assertTrue(process.destroyed)
        assertFalse("the embedded store is deleted, not left behind", dataDir.exists())
        assertNull(server.socketPathIfRunning())
    }
}
