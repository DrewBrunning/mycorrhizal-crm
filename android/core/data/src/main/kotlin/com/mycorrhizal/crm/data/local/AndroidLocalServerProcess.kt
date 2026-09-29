package com.mycorrhizal.crm.data.local

import android.util.Log
import org.json.JSONObject
import java.io.BufferedReader
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicReference

/**
 * The production [LocalServerProcessLauncher]: execs the packaged Go binary with
 * a cleared environment (non-secret values only, from the command) and the
 * HostConfig JSON on stdin — the secrets' only path to the child. # pragma: no
 * cover — this is the real `ProcessBuilder` path; the host's start/stop logic is
 * covered through a fake launcher, and the wire contract through host_test.go.
 */
class AndroidLocalServerProcessLauncher : LocalServerProcessLauncher {
    override fun launch(command: LocalServerCommand): LocalServerProcess =
        AndroidLocalServerProcess(command)
}

internal class AndroidLocalServerProcess(command: LocalServerCommand) : LocalServerProcess {

    private val process: Process
    private val readyLatch = CountDownLatch(1)
    private val sessionToken = AtomicReference<String?>(null)

    init {
        val builder = ProcessBuilder(listOf(command.binaryPath) + command.args)
            .directory(java.io.File(command.workDir))
            .redirectErrorStream(false)
        // Start from a clean environment so nothing from the app process (which
        // has held the unwrapped secrets in memory) is inherited. The child gets
        // only these non-secret values; the secrets arrive on stdin.
        builder.environment().clear()
        builder.environment().putAll(command.environment)

        process = builder.start()

        // The config goes on stdin, then EOF. Done before the reader threads so
        // the child can parse it while we wait for the handshake.
        process.outputStream.use { out ->
            out.write(command.stdinPayload.toByteArray())
            out.flush()
        }

        Thread({ pump(process.inputStream, isStdout = true) }, "local-server-stdout").apply {
            isDaemon = true
            start()
        }
        Thread({ pump(process.errorStream, isStdout = false) }, "local-server-stderr").apply {
            isDaemon = true
            start()
        }
    }

    // A dropped pipe (the child exiting) surfaces as an arbitrary
    // IOException/runtime type; this is the process boundary, so every failure
    // just ends the drain thread.
    @Suppress("TooGenericExceptionCaught")
    private fun pump(stream: java.io.InputStream, isStdout: Boolean) {
        try {
            BufferedReader(stream.reader()).useLines { lines ->
                lines.forEach { line ->
                    if (isStdout && sessionToken.get() == null && line.contains(HOST_READY_KEY)) {
                        runCatching {
                            val json = JSONObject(line)
                            if (json.optBoolean("host_ready")) {
                                sessionToken.set(json.optString("session_token").ifBlank { null })
                                readyLatch.countDown()
                                return@forEach
                            }
                        }
                    }
                    Log.i(TAG, line)
                }
            }
        } catch (t: Throwable) {
            Log.w(TAG, "server output stream ended: ${t.message}")
        } finally {
            readyLatch.countDown()
        }
    }

    override fun isAlive(): Boolean = process.isAlive

    override fun destroy() {
        process.destroy()
        if (!process.waitFor(2, TimeUnit.SECONDS)) {
            process.destroyForcibly()
        }
    }

    override fun awaitSessionToken(timeoutMs: Long): String? {
        readyLatch.await(timeoutMs, TimeUnit.MILLISECONDS)
        return sessionToken.get()
    }

    private companion object {
        const val TAG = "LocalServerProcess"
        const val HOST_READY_KEY = "host_ready"
    }
}
