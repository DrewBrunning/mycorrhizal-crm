package com.mycorrhizal.crm

import android.app.Application
import dagger.hilt.android.HiltAndroidApp

@HiltAndroidApp
class MycorrhizalApplication : Application() {
    override fun onCreate() {
        super.onCreate()
    }
}
