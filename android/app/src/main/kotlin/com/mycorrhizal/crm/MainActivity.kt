package com.mycorrhizal.crm

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.runtime.getValue
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import dagger.hilt.android.AndroidEntryPoint

@AndroidEntryPoint
class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContent {
            val darkTheme = isSystemInDarkTheme()
            MycorrhizalTheme(darkTheme = darkTheme) {
                MycorrhizalApp()
            }
        }
    }
}
