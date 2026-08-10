package com.mycorrhizal.crm

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.runtime.getValue
import androidx.core.view.WindowCompat
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.ui.theme.MycorrhizalColors
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import dagger.hilt.android.AndroidEntryPoint

@AndroidEntryPoint
class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        // Nav bar: brand green with light icons; status bar: transparent
        // (content draws under it, scaffold's surface shows through).
        window.navigationBarColor = MycorrhizalColors.mycelium.toArgbCompat()
        window.statusBarColor = android.graphics.Color.TRANSPARENT
        WindowCompat.getInsetsController(window, window.decorView).apply {
            isAppearanceLightNavigationBars = false
            isAppearanceLightStatusBars = false
        }
        setContent {
            val darkTheme = isSystemInDarkTheme()
            MycorrhizalTheme(darkTheme = darkTheme) {
                MycorrhizalApp()
            }
        }
    }

    private fun androidx.compose.ui.graphics.Color.toArgbCompat(): Int =
        android.graphics.Color.argb(
            (alpha * 255).toInt(),
            (red * 255).toInt(),
            (green * 255).toInt(),
            (blue * 255).toInt(),
        )
}
