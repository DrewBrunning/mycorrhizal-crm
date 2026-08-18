package com.mycorrhizal.crm

import android.content.Context
import android.content.Intent
import android.content.res.Configuration
import android.net.Uri
import android.os.Bundle
import android.widget.Toast
import androidx.activity.ComponentActivity
import androidx.activity.SystemBarStyle
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.runtime.getValue
import androidx.core.splashscreen.SplashScreen.Companion.installSplashScreen
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.lifecycleScope
import com.mycorrhizal.crm.data.repository.AppLocale
import com.mycorrhizal.crm.data.session.SessionManager
import com.mycorrhizal.crm.domain.repository.AppSettingsRepository
import com.mycorrhizal.crm.domain.repository.AuthRepository
import com.mycorrhizal.crm.domain.repository.SessionState
import com.mycorrhizal.crm.feature.tracking.NotificationBuilder
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.theme.MycorrhizalColors
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import com.mycorrhizal.crm.ui.util.LocaleContextWrapper
import dagger.hilt.android.AndroidEntryPoint
import javax.inject.Inject
import kotlinx.coroutines.launch

private fun androidx.compose.ui.graphics.Color.toArgbCompat(): Int =
    android.graphics.Color.argb(
        (alpha * 255).toInt(),
        (red * 255).toInt(),
        (green * 255).toInt(),
        (blue * 255).toInt(),
    )

@AndroidEntryPoint
class MainActivity : ComponentActivity() {

    @Inject
    lateinit var appSettings: AppSettingsRepository

    // M5 §5: the OIDC native return stores the JWT so the session flips to
    // logged-in without a second manual login (the web sets an httpOnly cookie
    // it cannot hand to bearer-token Android).
    @Inject
    lateinit var sessionManager: SessionManager

    @Inject
    lateinit var authRepository: AuthRepository

    // M5 §6.6 (issue #152): the most recent notification deep link to route the
    // NavHost to. Notifications carry it as an extra on their content intent;
    // onCreate/onNewIntent store it here and MycorrhizalApp consumes it (and
    // calls onDeepLinkHandled to reset). A StateFlow so a deep link that lands
    // while the main tree isn't composed yet (e.g. while still logged out) is
    // still delivered once the NavHost exists.
    private val pendingDeepLink = kotlinx.coroutines.flow.MutableStateFlow<android.net.Uri?>(null)

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

        // M5 §5: handle a cold-start OIDC deep link before the first frame so
        // the session is already present when the app tree composes.
        handleOidcReturn(intent?.data)
        // M5 §6.6: a notification tap's deep link arrives on the launch intent.
        handleNotificationDeepLink(intent)

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
                )
            }
        }
    }

    // singleTask: a deep link to the running activity arrives here, not in
    // onCreate — handle it the same way.
    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        handleOidcReturn(intent.data)
        handleNotificationDeepLink(intent)
    }

    /**
     * M5 §6.6 (issue #152): a notification tap's content intent is the app's
     * launch intent carrying [NotificationBuilder.EXTRA_DEEP_LINK] (a
     * `mycorrhizal://…` URI). Forward it to the NavHost; see [deepLinkRoute].
     */
    private fun handleNotificationDeepLink(intent: Intent?) {
        val link = intent?.getStringExtra(NotificationBuilder.EXTRA_DEEP_LINK)
        if (!link.isNullOrBlank()) {
            pendingDeepLink.value = Uri.parse(link)
        }
    }

    /**
     * M5 §5: the backend's OIDC callback redirect. Success carries
     * `token` + `language` + `date_format`; failure carries `error`. The token
     * is stored with the server URL the login screen already persisted, and
     * the session flow flips the app to logged-in. No httpOnly cookie is
     * minted for the native flow (M6 §4), so bearer-token Android must capture
     * the JWT itself.
     */
    private fun handleOidcReturn(uri: Uri?) {
        when (val parsed = parseOidcReturn(uri)) {
            is OidcReturn.Failure -> {
                Toast.makeText(this, getString(R.string.oidc_login_failed), Toast.LENGTH_LONG).show()
            }
            is OidcReturn.Success -> {
                val result = parsed
                lifecycleScope.launch {
                    // The persisted session hydrates asynchronously at startup;
                    // a cold-start deep link must await it or it'd read a null
                    // server URL and drop the JWT (review-pass fix).
                    sessionManager.awaitHydrated()
                    val serverUrl = sessionManager.serverUrl()
                    if (serverUrl.isNullOrBlank()) return@launch
                    sessionManager.setSession(
                        serverUrl = serverUrl,
                        token = result.token,
                        state = SessionState(language = result.language, dateFormat = result.dateFormat),
                    )
                    // Validate the JWT against the server and enrich the
                    // profile the way a normal login does (user id/username/
                    // admin for Settings); a stale/expired token never flips
                    // the app to logged-in (review-pass fix).
                    val profile = authRepository.fetchCurrentUser()
                    profile.fold(
                        onSuccess = { user ->
                            sessionManager.setProfile(
                                SessionState(
                                    userId = user.id.takeIf { it != 0 },
                                    username = user.username,
                                    isAdmin = user.isAdmin,
                                ),
                            )
                        },
                        onFailure = {
                            sessionManager.clearSession()
                            Toast.makeText(this@MainActivity, getString(R.string.oidc_login_failed), Toast.LENGTH_LONG).show()
                        },
                    )
                }
            }
            null -> Unit
        }
    }
}

/**
 * Parses a notification deep link (`mycorrhizal://contacts/{id}`) into a NavHost
 * route (`contacts/{id}`). Null when the URI is not a route the app knows — a
 * malformed or foreign link should never drive navigation. Pure and internal so
 * it is unit-testable without an Activity.
 */
internal fun deepLinkRoute(uri: Uri?): String? {
    if (uri == null || uri.scheme != "mycorrhizal" || uri.host != "contacts") return null
    val id = uri.path?.trimStart('/')?.toIntOrNull() ?: return null
    if (id <= 0) return null
    return "contacts/$id"
}

/**
 * Parses the OIDC native-return deep link (`mycorrhizal://oidc/callback`).
 * Pure and internal so it is unit-testable without an Activity.
 * [OidcReturn.Success] carries the captured JWT (bearer-token Android's
 * replacement for web's httpOnly cookie); [OidcReturn.Failure] carries the
 * backend's error; null means the URI is not this deep link.
 */
internal sealed interface OidcReturn {
    data class Success(val token: String, val language: String?, val dateFormat: String?) : OidcReturn
    data object Failure : OidcReturn
}

internal fun parseOidcReturn(uri: Uri?): OidcReturn? {
    if (uri == null || uri.scheme != "mycorrhizal" || uri.host != "oidc") return null
    // Path check too, not just host: MainActivity is exported, so any app on
    // the device could otherwise hit us with an explicit-component VIEW intent
    // (review-pass fix).
    if (uri.path != "/callback") return null
    if (uri.getQueryParameter("error") != null) return OidcReturn.Failure
    val token = uri.getQueryParameter("token")
    if (token.isNullOrBlank()) return null
    return OidcReturn.Success(
        token = token,
        language = uri.getQueryParameter("language"),
        dateFormat = uri.getQueryParameter("date_format"),
    )
}
