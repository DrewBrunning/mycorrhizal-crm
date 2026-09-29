package com.mycorrhizal.crm

import android.content.Context
import android.content.Intent
import android.content.res.Configuration
import android.net.Uri
import android.os.Bundle
import android.view.WindowManager
import androidx.activity.SystemBarStyle
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.runtime.getValue
import androidx.core.splashscreen.SplashScreen.Companion.installSplashScreen
import androidx.fragment.app.FragmentActivity
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.lifecycleScope
import com.mycorrhizal.crm.data.repository.AppLocale
import com.mycorrhizal.crm.data.session.OidcPendingRequestStore
import com.mycorrhizal.crm.data.session.SessionManager
import com.mycorrhizal.crm.domain.repository.AppSettingsRepository
import com.mycorrhizal.crm.domain.repository.AuthRepository
import com.mycorrhizal.crm.domain.repository.PendingInteractionRepository
import com.mycorrhizal.crm.domain.repository.SessionState
import com.mycorrhizal.crm.domain.repository.TrackingSettingsRepository
import com.mycorrhizal.crm.feature.tracking.NotificationBuilder
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.theme.MycorrhizalColors
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import com.mycorrhizal.crm.ui.util.LocaleContextWrapper
import dagger.hilt.android.AndroidEntryPoint
import javax.inject.Inject
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.launch

private fun androidx.compose.ui.graphics.Color.toArgbCompat(): Int =
    android.graphics.Color.argb(
        (alpha * 255).toInt(),
        (red * 255).toInt(),
        (green * 255).toInt(),
        (blue * 255).toInt(),
    )

@AndroidEntryPoint
// Issue #722: FragmentActivity (still a ComponentActivity, so the Compose
// setContent/edge-to-edge extensions are unchanged) is the host type
// androidx.biometric's BiometricPrompt requires. No Fragment is ever added —
// this is purely the prompt's host contract.
class MainActivity : FragmentActivity() {

    @Inject
    lateinit var appSettings: AppSettingsRepository

    // Public (like sessionManager) so the instrumented E2E suite can arm and
    // release the app-lock gate through the app's real controller (issue #1269).
    @Inject
    lateinit var appLockController: com.mycorrhizal.crm.data.session.AppLockController

    @Inject
    lateinit var localAuthSettings: com.mycorrhizal.crm.domain.repository.LocalAuthSettingsRepository

    // M5 §5: the OIDC native return stores the JWT so the session flips to
    // logged-in without a second manual login (the web sets an httpOnly cookie
    // it cannot hand to bearer-token Android).
    @Inject
    lateinit var sessionManager: SessionManager

    @Inject
    lateinit var authRepository: AuthRepository

    // Issue #965: the state nonce + PKCE verifier for an in-flight native OIDC
    // login. Written when the flow starts, read (and cleared) when the deep
    // link returns; persisted so a cold-start return after the app was
    // backgrounded can still verify and redeem.
    @Inject
    lateinit var oidcPendingStore: OidcPendingRequestStore

    // Exposed purely for the instrumented E2E suite (issue #238), the same way
    // sessionManager is above: ANDROID-04 (#481) drives DeviceRegistrationManager
    // directly against this already-authenticated singleton to exercise the FCM
    // device-registration lifecycle against the real backend.
    @Inject
    lateinit var apiClient: ApiClient

    // ANDROID-02 (issue #479): exposed purely for the offline-sync E2E suite —
    // it queues interactions through the real outbox repository (the same call
    // the call-log/SMS capture paths make) and drives the real periodic sync
    // worker against the real backend, rather than exercising a mocked copy.
    @Inject
    lateinit var pendingInteractionRepository: PendingInteractionRepository

    // ANDROID-02 (issue #479): the same suite enables the call/SMS tracking
    // opt-ins through the real DataStore-backed repository the worker itself
    // reads (InteractionSyncWorker gates on these), so the E2E run is the real
    // production gate, not a test-only backdoor.
    @Inject
    lateinit var trackingSettings: TrackingSettingsRepository

    // M5 §6.6 (issue #152): the most recent notification deep link to route the
    // NavHost to. Notifications carry it as an extra on their content intent;
    // onCreate/onNewIntent store it here and MycorrhizalApp consumes it (and
    // calls onDeepLinkHandled to reset). A StateFlow so a deep link that lands
    // while the main tree isn't composed yet (e.g. while still logged out) is
    // still delivered once the NavHost exists.
    //
    // ADR 0029 §3–4 (issue #1268): each link is stamped with its arrival time so
    // the consumer can drop one that outlived [DEEP_LINK_TTL_MILLIS], and the
    // flow is cleared whenever the session flips to logged-out so a link fired
    // while signed out cannot navigate in whichever account signs in next.
    private val pendingDeepLink = MutableStateFlow<PendingDeepLink?>(null)

    // #203 (issue #203): the OIDC-return failure message used to be a Toast,
    // which is announced inconsistently by TalkBack and can't be re-read.
    // Both failure paths in handleOidcReturn end with the session logged out,
    // so this is consumed by LoginScreen's existing SnackbarHostState — same
    // StateFlow shape as pendingDeepLink above, for the same reason: a
    // cold-start failure fires before setContent{} composes anything.
    private val oidcError = kotlinx.coroutines.flow.MutableStateFlow<String?>(null)

    // M25: the user's chosen UI language must reach the very first frame.
    // attachBaseContext runs before Hilt injection, so the locale comes from
    // the synchronous AppLocale cache (hydrated at startup, updated whenever
    // the setting changes). A language change recreates the Activity so this
    // re-runs with the fresh value.
    override fun attachBaseContext(newBase: Context) {
        super.attachBaseContext(LocaleContextWrapper.wrap(newBase, AppLocale.languageTag))
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        // T106: must precede super.onCreate() -- required by the library, not stylistic.
        installSplashScreen()
        super.onCreate(savedInstanceState)

        // Issue #507 (MASVS-L2 resilience re-evaluation): every screen in this
        // single-Activity app renders relationship PII, so both are applied
        // unconditionally rather than per-screen.
        //
        // FLAG_SECURE blocks screenshots/screen recording and blanks the
        // recent-apps thumbnail — the thumbnail case is the concrete threat
        // (anyone with a moment's physical access to an unlocked phone can
        // open the app switcher without unlocking the app itself). Reverses
        // masvs-l1.md P3 for android_prevent_screenshot; see that file and
        // threat-model.md gating decision 3 for the full cost/benefit writeup.
        window.setFlags(WindowManager.LayoutParams.FLAG_SECURE, WindowManager.LayoutParams.FLAG_SECURE)

        // filterTouchesWhenObscured rejects touches while another app's window
        // overlays this one (a tapjacking/overlay attack tricking the user into
        // tapping something they can't see, e.g. a "grant permission" or
        // "confirm delete" control). Set on the decor view so it gates
        // dispatch before it reaches any child — no per-screen wiring needed,
        // and it has zero effect on normal single-window use. Reverses
        // masvs-l1.md P3 for android_detect_tapjacking/android_tapjacking.
        window.decorView.filterTouchesWhenObscured = true

        // ADR 0029 §4: a link never outlives the session that was signed in
        // when it arrived.
        lifecycleScope.launch {
            pendingDeepLink.clearWhenLoggedOut(sessionManager.observeSession())
        }

        // M5 §5: handle a cold-start OIDC deep link before the first frame so
        // the session is already present when the app tree composes.
        // M5 §6.6: a notification tap's deep link arrives on the launch intent.
        // ADR 0029 §3 (issue #1268): consume once — a recreate (language change,
        // savedInstanceState != null) or a reopen from Recents re-delivers the
        // original launch intent, which must not be handled a second time.
        consumeLaunchIntent(savedStateIsNull = savedInstanceState == null)

        // M25: the theme preference is a live local setting (system/light/dark),
        // so darkThemeAtLaunch follows it instead of the bare system config when
        // it has been pinned. AppLocale is the sync cache (see attachBaseContext).
        val darkThemeAtLaunch = when (appSettings.currentThemePreference()) {
            AppSettingsRepository.THEME_DARK -> true
            AppSettingsRepository.THEME_LIGHT -> false
            else -> (resources.configuration.uiMode and Configuration.UI_MODE_NIGHT_MASK) ==
                Configuration.UI_MODE_NIGHT_YES
        }

        enableEdgeToEdge(
            statusBarStyle = if (darkThemeAtLaunch) {
                // myceliumDark is light-toned (M3 dark-scheme accents are lighter than their
                // light-scheme counterpart) -> dark icons. SystemBarStyle.light always forces
                // dark icons regardless of system dark mode, so the second (darkScrim) param
                // is unused here; passed the same color for clarity.
                SystemBarStyle.light(
                    MycorrhizalColors.myceliumDark.toArgbCompat(),
                    MycorrhizalColors.myceliumDark.toArgbCompat(),
                )
            } else {
                SystemBarStyle.dark(MycorrhizalColors.mycelium.toArgbCompat())
            },
            navigationBarStyle = if (darkThemeAtLaunch) {
                SystemBarStyle.dark(MycorrhizalColors.boneDark.toArgbCompat())
            } else {
                SystemBarStyle.light(
                    MycorrhizalColors.bone.toArgbCompat(),
                    MycorrhizalColors.bone.toArgbCompat(),
                )
            },
        )

        setContent {
            // M25: theme is a live setting, not only the system default. The
            // Flow gives us recomposition when it changes (MainActivity is
            // not recreated on a theme change, only on a language change).
            val themePreference by appSettings.themePreference()
                .collectAsStateWithLifecycle(initialValue = appSettings.currentThemePreference())
            val darkTheme = when (themePreference) {
                AppSettingsRepository.THEME_DARK -> true
                AppSettingsRepository.THEME_LIGHT -> false
                else -> isSystemInDarkTheme()
            }
            MycorrhizalTheme(darkTheme = darkTheme) {
                MycorrhizalApp(
                    darkTheme = darkTheme,
                    deepLinks = pendingDeepLink,
                    onDeepLinkHandled = { pendingDeepLink.value = null },
                    onStartOidc = ::startOidcLogin,
                    oidcError = oidcError,
                    onOidcErrorShown = { oidcError.value = null },
                )
            }
        }
    }

    // singleTask: a deep link to the running activity arrives here, not in
    // onCreate — handle it the same way.
    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        consumeLaunchIntent(savedStateIsNull = true)
    }

    /**
     * Handles the activity intent's OIDC return / notification link at most
     * once (ADR 0029 §3): skipped when [shouldHandleLaunchIntent] says it is a
     * replay, and the consumed data is stripped from the retained intent so any
     * later re-read (recreate, process restore) finds nothing to replay.
     */
    private fun consumeLaunchIntent(savedStateIsNull: Boolean) {
        val current = intent ?: return
        if (!shouldHandleLaunchIntent(savedStateIsNull, current.flags)) return
        handleOidcReturn(current.data)
        handleDeepLink(current)
        setIntent(Intent(current).apply {
            data = null
            removeExtra(NotificationBuilder.EXTRA_DEEP_LINK)
        })
    }

    /**
     * M5 §6.6 (issue #152) + ADR 0029 §2 (issue #1269): a deep link reaches the app
     * either as a public `ACTION_VIEW` intent's data (launchers, bookmarks,
     * automation) or, for a notification tap, as [NotificationBuilder.EXTRA_DEEP_LINK].
     * The OIDC callback is also a VIEW intent but is auth-only and handled by
     * [handleOidcReturn], never navigated. Both sources feed the same
     * [deepLinkRoute] (the security boundary; the manifest filter is advisory).
     */
    private fun handleDeepLink(intent: Intent?) {
        val link = deepLinkUri(intent) ?: return
        pendingDeepLink.value = PendingDeepLink(link, System.currentTimeMillis())
    }

    /**
     * M5 §5 / issue #965: the backend's OIDC callback redirect. Success carries
     * the short-lived `code` bound to the app's PKCE challenge plus the `state`
     * nonce and `language`/`date_format`; failure carries `error`. The
     * orchestration (state/TTL binding, code redemption, profile enrichment)
     * lives in [OidcLoginCoordinator] so it is unit-testable without an
     * Activity; this just parses and reports the outcome.
     */
    private fun handleOidcReturn(uri: Uri?) {
        lifecycleScope.launch {
            if (oidcLogin.onCallback(parseOidcReturn(uri)) == OidcCallbackOutcome.Failed) {
                oidcError.value = getString(R.string.oidc_login_failed)
            }
        }
    }

    /**
     * Issue #965: open the native OIDC flow. [OidcLoginCoordinator.start]
     * generates and persists the state + PKCE binding, then invokes this to
     * launch the browser.
     */
    private fun startOidcLogin(serverUrl: String) {
        lifecycleScope.launch { oidcLogin.start(serverUrl) }
    }

    /** Lazy so the injected fields above are set before it is built. */
    private val oidcLogin: OidcLoginCoordinator by lazy {
        OidcLoginCoordinator(
            sessionManager = sessionManager,
            authRepository = authRepository,
            pendingStore = oidcPendingStore,
            launchBrowser = { url ->
                val intent = Intent(Intent.ACTION_VIEW, Uri.parse(url))
                runCatching { startActivity(intent) }
            },
        )
    }
}

/**
 * Parses a notification deep link (`mycorrhizal://…`) into a NavHost route.
 * Returns null when the URI is not a route the app knows — a malformed or
 * foreign link should never drive navigation (ADR-0002: degrade, don't crash).
 *
 * Supported today (issue #679), each mapping to a route that actually exists
 * in the NavHost:
 *  - `mycorrhizal://home`                            → `home`
 *  - `mycorrhizal://contacts/{id}`                   → `contacts/{id}`
 *  - `mycorrhizal://contacts/{id}/activities`        → `contacts/{id}/activities`
 *  - `mycorrhizal://circles|tags|households/{id}`    → `circles|tags|households/{id}`
 *  - `mycorrhizal://search?q=…`                       → `contacts?search=…` (or `contacts`)
 *
 * Pure and internal so it is unit-testable without an Activity. The id rules
 * follow each destination's nav-argument type: contacts/activities take a
 * positive integer; circles/tags/households take a non-blank string id
 * (VCardUID). Parsing is strict (ADR 0029 §2, issue #1268). OIDC's `mycorrhizal://oidc/callback` is deliberately not a
 * navigable route (it is handled before navigation, in [MainActivity]).
 */
internal fun deepLinkRoute(uri: Uri?): String? {
    if (uri == null || uri.scheme != "mycorrhizal") return null
    val segments = strictPathSegments(uri) ?: return null
    return when (uri.host) {
        "home" -> if (segments.isEmpty()) "home" else null
        "search" -> if (segments.isEmpty()) searchRoute(uri) else null
        "contacts" -> contactsRoute(segments)?.let { "contacts/$it" }
        "circles", "tags", "households" ->
            if (segments.size == 1 && OPAQUE_ID.matches(segments[0])) "${uri.host}/${segments[0]}" else null
        else -> null
    }
}

/**
 * The deep-link URI carried by [intent]: a VIEW intent's data unless it is the
 * OIDC callback, else the notification extra. Null when there is none.
 */
internal fun deepLinkUri(intent: Intent?): Uri? {
    if (intent == null) return null
    val data = intent.data
    if (intent.action == Intent.ACTION_VIEW && data != null && parseOidcReturn(data) == null) return data
    return intent.getStringExtra(NotificationBuilder.EXTRA_DEEP_LINK)
        ?.takeIf { it.isNotBlank() }
        ?.let(Uri::parse)
}

private const val SEARCH_QUERY_MAX_CHARS = 200

/**
 * `mycorrhizal://search?q=…` → `contacts?search=<encoded q>` or `contacts` when q is
 * empty after sanitising (ADR 0029 §3): trim, strip Unicode Cc/Cf, truncate to 200.
 */
private fun searchRoute(uri: Uri): String {
    val q = (uri.getQueryParameter("q") ?: "")
        .trim()
        .filter { Character.getType(it) != Character.CONTROL.toInt() && Character.getType(it) != Character.FORMAT.toInt() }
        .take(SEARCH_QUERY_MAX_CHARS)
    return if (q.isEmpty()) "contacts" else "contacts?search=${Uri.encode(q)}"
}

/** `[1-9][0-9]{0,9}` — no sign, `+`, or leading zero (ADR 0029 §2). */
private val INTEGER_ID = Regex("^[1-9][0-9]{0,9}$")

/** Circle/tag/household ids (VCardUIDs): ADR 0029 §2's opaque-id alphabet. */
private val OPAQUE_ID = Regex("^[A-Za-z0-9._:-]{1,128}$")

/**
 * The decoded path segments of [uri], or null when the path is malformed: an
 * empty segment (`//`, trailing `/`), a `.`/`..` segment, or a segment that
 * decodes to contain `/` (an encoded `%2F` smuggling a separator). Reads the
 * *encoded* path because [Uri.getPathSegments] silently drops empty segments.
 */
private fun strictPathSegments(uri: Uri): List<String>? {
    val raw = uri.encodedPath.orEmpty().removePrefix("/")
    if (raw.isEmpty()) return emptyList()
    val decoded = raw.split('/').map { Uri.decode(it) }
    if (decoded.any { it.isEmpty() || it == "." || it == ".." || '/' in it }) return null
    return decoded
}

/**
 * Deep-link path under `mycorrhizal://contacts/…`. Returns `id` or
 * `id/activities`; null for a blank/malformed path or an unknown sub-route.
 */
private fun contactsRoute(segments: List<String>): String? {
    if (segments.isEmpty() || segments.size > 2) return null
    val idText = segments[0]
    if (!INTEGER_ID.matches(idText)) return null
    val id = idText.toLongOrNull()?.takeIf { it <= Int.MAX_VALUE } ?: return null
    return when (segments.size) {
        1 -> id.toString()
        else -> if (segments[1] == "activities") "$id/activities" else null
    }
}

/** ADR 0029 §4: a pending link older than this is dropped, not navigated. */
internal const val DEEP_LINK_TTL_MILLIS = 10 * 60 * 1000L

/** A deep link waiting for the NavHost, stamped so it can expire. */
data class PendingDeepLink(val uri: Uri, val receivedAtMillis: Long) {
    fun isExpired(nowMillis: Long): Boolean = nowMillis - receivedAtMillis > DEEP_LINK_TTL_MILLIS
}

/**
 * ADR 0029 §3: whether an incoming launch intent may be handled. False for a
 * recreate (saved state present) and for a reopen from Recents
 * (`FLAG_ACTIVITY_LAUNCHED_FROM_HISTORY`) — both re-deliver an intent that was
 * already consumed. Pure so it is testable without an Activity.
 */
internal fun shouldHandleLaunchIntent(savedStateIsNull: Boolean, flags: Int): Boolean =
    savedStateIsNull && (flags and Intent.FLAG_ACTIVITY_LAUNCHED_FROM_HISTORY) == 0

/**
 * ADR 0029 §4: clears this pending link whenever [session] reports logged-out,
 * so a link received while signed out never navigates in the next account.
 */
internal suspend fun MutableStateFlow<PendingDeepLink?>.clearWhenLoggedOut(session: Flow<SessionState>) {
    session.collect { if (!it.isLoggedIn) value = null }
}

/**
 * Parses the OIDC native-return deep link (`mycorrhizal://oidc/callback`).
 * Pure and internal so it is unit-testable without an Activity.
 *
 * Issue #965: [OidcReturn.Success] carries a short-lived, PKCE-bound exchange
 * `code` plus the app's `state` nonce — NOT the session JWT. The pre-#965
 * `token` parameter is deliberately not parsed; a deep link that supplies one
 * is ignored so the custom-scheme token theft it fixed cannot be reintroduced.
 * [OidcReturn.Failure] carries the backend's error; null means the URI is not
 * this deep link.
 */
internal sealed interface OidcReturn {
    data class Success(
        val state: String,
        val code: String,
        val language: String?,
        val dateFormat: String?,
    ) : OidcReturn

    data object Failure : OidcReturn
}

internal fun parseOidcReturn(uri: Uri?): OidcReturn? {
    if (uri == null || uri.scheme != "mycorrhizal" || uri.host != "oidc") return null
    // Path check too, not just host: MainActivity is exported, so any app on
    // the device could otherwise hit us with an explicit-component VIEW intent
    // (review-pass fix).
    if (uri.path != "/callback") return null
    if (uri.getQueryParameter("error") != null) return OidcReturn.Failure
    // #965: the code + state are the exchange contract. A raw token (the old
    // shape) is not accepted — only a code redeemable with the on-device PKCE
    // verifier is.
    val code = uri.getQueryParameter("code")
    val state = uri.getQueryParameter("state")
    if (code.isNullOrBlank() || state.isNullOrBlank()) return null
    return OidcReturn.Success(
        state = state,
        code = code,
        language = uri.getQueryParameter("language"),
        dateFormat = uri.getQueryParameter("date_format"),
    )
}
