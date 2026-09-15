package com.mycorrhizal.crm.data.session

import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Guard test for the 401/session-expiry wiring (issue #678). The DI graph
 * cannot be booted on the JVM, so — following [EncryptedTokenStorageGuardTest]
 * and [RoomEncryptionGuardTest]'s pattern — this asserts on the source: it
 * fails if the session manager stops wiring [SessionExpiryWiring] to the
 * notifier, if the app's OkHttp chain drops [SessionExpiryInterceptor], or if
 * the wiring stops calling `clearSession` on the signal. The behavioral core
 * is covered directly by [SessionExpiryInterceptorTest] and
 * [SessionExpiryWiringTest].
 */
class SessionExpiryWiringGuardTest {

    private val dataModuleSource =
        File("src/main/kotlin/com/mycorrhizal/crm/data/di/DataModule.kt").readText()

    private val wiringSource =
        File("src/main/kotlin/com/mycorrhizal/crm/data/session/SessionExpiryWiring.kt").readText()

    private val appNetworkModuleSource =
        File("../../app/src/main/kotlin/com/mycorrhizal/crm/di/AppNetworkModule.kt").readText()

    private val sessionTeardownModuleSource =
        File("../../app/src/main/kotlin/com/mycorrhizal/crm/di/SessionTeardownModule.kt").readText()

    @Test
    fun `the session manager is wired to the session-expiry notifier`() {
        assertTrue(
            "DataModule must provide a SessionExpiryNotifier",
            dataModuleSource.contains("fun provideSessionExpiryNotifier"),
        )
        assertTrue(
            "provideSessionManager must start SessionExpiryWiring",
            dataModuleSource.contains("SessionExpiryWiring(") &&
                dataModuleSource.contains(".start(scope)"),
        )
        // Issue #722: the wiring must first try a device-grant exchange so an
        // expired-but-valid session resumes on a biometric-enrolled device.
        assertTrue(
            "the 401 path must attempt a device-grant refresh before clearing",
            dataModuleSource.contains("refresher = { deviceGrantManager.get().refreshSessionFromStoredGrant() }"),
        )
        // Issue #957: DefaultSessionManager must actually receive the
        // Hilt-bound SessionTeardown, not silently fall back to the Noop
        // default -- that would resurrect the original bug even though the
        // ordering fix in DefaultSessionManager itself is correct. A plain
        // (non-Provider) SessionTeardown parameter here is a real Dagger
        // DependencyCycle (confirmed by hand: SessionTeardown -> ApiClient ->
        // OkHttpClient -> TokenProvider -> SessionManager), so this also
        // pins the Provider indirection.
        assertTrue(
            "provideSessionManager must take a Provider<SessionTeardown>, not SessionTeardown directly",
            dataModuleSource.contains("sessionTeardown: javax.inject.Provider<SessionTeardown>"),
        )
        assertTrue(
            "provideSessionManager must pass the resolved SessionTeardown into DefaultSessionManager",
            dataModuleSource.contains("sessionTeardown = SessionTeardown { sessionTeardown.get().beforeClear() }"),
        )
    }

    @Test
    fun `a session-expiry signal clears the session`() {
        assertTrue(
            "SessionExpiryWiring must register a listener on the notifier",
            wiringSource.contains("sessionExpiryNotifier.register"),
        )
        assertTrue(
            "the registered listener must clear the session",
            wiringSource.contains("sessionManager.clearSession()"),
        )
        // Issue #957 (finding #1, point 2): a 401 from clearSession's own
        // authenticated teardown call must not attempt a refresh -- that
        // would silently resurrect the session already being torn down.
        assertTrue(
            "the listener must skip a refresh attempt while a clearSession call is already tearing down",
            wiringSource.contains("sessionManager.isClearingSession()"),
        )
    }

    @Test
    fun `the app's OkHttp chain carries the session-expiry interceptor`() {
        assertTrue(
            "AppNetworkModule must build a SessionExpiryInterceptor with the base-url host check",
            appNetworkModuleSource.contains("SessionExpiryInterceptor(sessionExpiryNotifier, baseUrlProvider)"),
        )
        assertTrue(
            "AppNetworkModule must pass the interceptor into NetworkFactory",
            appNetworkModuleSource.contains("sessionExpiryInterceptor ="),
        )
    }

    // Issue #957: core:data only knows the SessionTeardown interface (FCM
    // deregistration lives in feature:tracking, which core:data cannot
    // depend on) -- the real implementation must be Hilt-bound somewhere the
    // DI graph can see, or provideSessionManager silently falls back to
    // whatever default it declares (today NoopSessionTeardown), and logout
    // regresses to the original bug with no compile error to catch it.
    @Test
    fun `the real SessionTeardown is bound and composes both teardown steps`() {
        assertTrue(
            "SessionTeardownModule must bind SessionTeardown to a real implementation",
            sessionTeardownModuleSource.contains("abstract fun bindSessionTeardown(") &&
                sessionTeardownModuleSource.contains(": SessionTeardown"),
        )
        assertTrue(
            "the implementation must deregister the FCM device",
            sessionTeardownModuleSource.contains("deviceRegistration.delete()"),
        )
        assertTrue(
            "the implementation must revoke the server-side session",
            sessionTeardownModuleSource.contains("sessionRevoker.revoke()"),
        )
    }
}
