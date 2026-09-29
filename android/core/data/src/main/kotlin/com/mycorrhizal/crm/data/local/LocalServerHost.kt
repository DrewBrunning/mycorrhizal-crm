package com.mycorrhizal.crm.data.local

import android.content.Context
import android.os.Build
import com.mycorrhizal.crm.model.MoshiProvider
import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import java.io.File
import java.io.InputStream
import javax.inject.Inject
import javax.inject.Singleton

/**
 * ADR 0028 Decision 2 / issue #1262: the running embedded server. A `Local`
 * profile's lifecycle — start lazily on the first caller that needs it, serve
 * over the app-private Unix socket, stop on process death or background — is
 * owned here and nowhere else.
 */
interface LocalServerHost {
    /**
     * Starts the embedded server if it is not already running and returns its
     * endpoint. Idempotent and safe to call concurrently: the first caller
     * starts it, the rest join that start. Any worker or screen about to make a
     * request for a `Local` profile calls this first.
     */
    suspend fun ensureStarted(): Result<LocalServerEndpoint>

    /** Stops the server process. A no-op when it is not running. */
    suspend fun stop()

    /**
     * Stops the server and deletes its data directory and Keystore-wrapped
     * keys. This is the only copy of a `Local` profile's data, so callers must
     * confirm before invoking it (ADR 0028 Decision 1).
     */
    suspend fun deleteLocalData()

    /** The socket path while the server is running, else null (transport seam). */
    fun socketPathIfRunning(): String?

    /** The token the running server minted for its single local user, else null. */
    fun sessionTokenIfRunning(): String?
}

/** A running embedded server's transport and credential. */
data class LocalServerEndpoint(
    val socketPath: String,
    val sessionToken: String,
)

/** The app-private paths the embedded server owns. All under `filesDir`. */
data class LocalServerPaths(
    val dataDir: File,
    val socketPath: String,
    val dbPath: String,
    val photoDir: File,
    val attachmentsDir: File,
) {
    companion object {
        fun under(filesDir: File): LocalServerPaths {
            val dataDir = File(filesDir, "local-server")
            return LocalServerPaths(
                dataDir = dataDir,
                socketPath = File(dataDir, "sock").absolutePath,
                dbPath = File(dataDir, "mycorrhizal.db").absolutePath,
                photoDir = File(dataDir, "photos"),
                attachmentsDir = File(dataDir, "attachments"),
            )
        }
    }
}

/** Locates the packaged Go server binary, or null when this ABI has none. */
fun interface LocalServerBinaryProvider {
    fun binary(): File?
}

/** Launches the server process. A seam so tests need no real exec. */
fun interface LocalServerProcessLauncher {
    fun launch(command: LocalServerCommand): LocalServerProcess
}

/** Everything needed to exec the packaged server. */
data class LocalServerCommand(
    val binaryPath: String,
    /** argv after the binary; must select the Go side's `--embedded-host` entry point. */
    val args: List<String>,
    val workDir: String,
    val environment: Map<String, String>,
    /** The HostConfig JSON written to the child's stdin — the secrets' only path. */
    val stdinPayload: String,
)

/** A launched server process. */
interface LocalServerProcess {
    fun isAlive(): Boolean

    fun destroy()

    /**
     * Blocks up to [timeoutMs] for the child's readiness handshake (the
     * `{"host_ready":true,"session_token":"…"}` line) and returns the token, or
     * null on timeout/exit. The production launcher keeps draining the child's
     * output to the platform log in the background; the host only needs the
     * handshake.
     */
    fun awaitSessionToken(timeoutMs: Long): String?
}

/** Probes the embedded server's `/health` over the Unix socket. */
fun interface LocalServerHealthProbe {
    suspend fun isHealthy(socketPath: String): Boolean
}

/** Whether this device/ABI can run the embedded server at all (ADR 0028, arm64-only). */
object LocalServerAvailability {
    const val BINARY_NAME = "libmycorrhizal.so"
    const val REQUIRED_ABI = "arm64-v8a"

    fun isSupported(context: Context): Boolean =
        Build.SUPPORTED_ABIS.contains(REQUIRED_ABI) &&
            File(context.applicationInfo.nativeLibraryDir, BINARY_NAME).exists()
}

/** The HostConfig JSON written to the child's stdin. Field names mirror `embedded.HostConfig`. */
@JsonClass(generateAdapter = true)
internal data class LocalServerHostPayload(
    @Json(name = "jwt_secret") val jwtSecret: String,
    @Json(name = "data_encryption_key") val dataEncryptionKey: String,
    @Json(name = "socket_path") val socketPath: String,
    @Json(name = "db_path") val dbPath: String,
    @Json(name = "profile_photo_dir") val profilePhotoDir: String,
    @Json(name = "attachments_dir") val attachmentsDir: String,
    @Json(name = "catch_up_delay_ms") val catchUpDelayMs: Long,
)

/**
 * The production host. The start sequence is: ensure directories, load-or-create
 * the Keystore-wrapped secrets, exec the packaged binary with the non-secret
 * values in the environment and the whole HostConfig (secrets included) on
 * stdin, await the readiness handshake for the session token, then confirm
 * `/health` answers before declaring the endpoint usable. Any failure destroys
 * the process and leaves the host clean for a retry.
 */
@Singleton
class AndroidLocalServerHost @Inject constructor(
    private val paths: LocalServerPaths,
    private val binaryProvider: LocalServerBinaryProvider,
    private val secretStore: LocalServerSecretStore,
    private val launcher: LocalServerProcessLauncher,
    private val healthProbe: LocalServerHealthProbe,
) : LocalServerHost {

    private val mutex = Mutex()

    @Volatile
    private var process: LocalServerProcess? = null

    @Volatile
    private var endpoint: LocalServerEndpoint? = null

    override suspend fun ensureStarted(): Result<LocalServerEndpoint> = mutex.withLock {
        endpoint?.takeIf { process?.isAlive() == true }?.let { return Result.success(it) }
        // A dead process or a stale endpoint: drop both and start fresh.
        clearLocked()

        val binary = binaryProvider.binary()
            ?: return Result.failure(IllegalStateException("embedded server binary is not packaged for this ABI"))

        runCatching {
            paths.dataDir.mkdirs()
            paths.photoDir.mkdirs()
            paths.attachmentsDir.mkdirs()

            val secrets = withContext(Dispatchers.IO) { secretStore.loadOrCreate() }
            val payload = LocalServerHostPayload(
                jwtSecret = secrets.jwtSecret,
                dataEncryptionKey = secrets.dataEncryptionKey,
                socketPath = paths.socketPath,
                dbPath = paths.dbPath,
                profilePhotoDir = paths.photoDir.absolutePath,
                attachmentsDir = paths.attachmentsDir.absolutePath,
                catchUpDelayMs = CATCH_UP_DELAY_MS,
            )
            val stdinJson = MoshiProvider.get().adapter(LocalServerHostPayload::class.java).toJson(payload)

            val command = LocalServerCommand(
                binaryPath = binary.absolutePath,
                args = listOf(EMBEDDED_HOST_ARG),
                workDir = paths.dataDir.absolutePath,
                environment = mapOf(
                    "HOME" to paths.dataDir.absolutePath,
                    "TMPDIR" to paths.dataDir.absolutePath,
                ),
                stdinPayload = stdinJson,
            )
            val launched = withContext(Dispatchers.IO) { launcher.launch(command) }
            process = launched

            val token = withContext(Dispatchers.IO) { launched.awaitSessionToken(READY_TIMEOUT_MS) }
                ?: error("embedded server did not report readiness")

            val healthy = withContext(Dispatchers.IO) { awaitHealthy() }
            check(healthy) { "embedded server socket never answered /health" }

            LocalServerEndpoint(socketPath = paths.socketPath, sessionToken = token).also { endpoint = it }
        }.onFailure {
            withContext(Dispatchers.IO) { process?.destroy() }
            clearLocked()
        }
    }

    override suspend fun stop() {
        mutex.withLock {
            withContext(Dispatchers.IO) { process?.destroy() }
            clearLocked()
        }
    }

    override suspend fun deleteLocalData() {
        stop()
        withContext(Dispatchers.IO) {
            secretStore.delete()
            paths.dataDir.deleteRecursively()
        }
    }

    override fun socketPathIfRunning(): String? = endpoint?.takeIf { process?.isAlive() == true }?.socketPath

    override fun sessionTokenIfRunning(): String? = endpoint?.takeIf { process?.isAlive() == true }?.sessionToken

    private fun clearLocked() {
        process = null
        endpoint = null
    }

    private suspend fun awaitHealthy(): Boolean {
        val deadline = System.currentTimeMillis() + HEALTH_TIMEOUT_MS
        while (System.currentTimeMillis() < deadline) {
            if (healthProbe.isHealthy(paths.socketPath)) return true
            if (process?.isAlive() == false) return false
            Thread.sleep(HEALTH_POLL_MS)
        }
        return false
    }

    private companion object {
        /** Defer the boot catch-up burst past the first request (spike #1256). */
        const val CATCH_UP_DELAY_MS = 3_000L
        const val READY_TIMEOUT_MS = 120_000L
        const val HEALTH_TIMEOUT_MS = 30_000L
        const val HEALTH_POLL_MS = 50L
    }
}

/** Must match `embeddedHostArg` in backend/main.go. */
internal const val EMBEDDED_HOST_ARG = "--embedded-host"
