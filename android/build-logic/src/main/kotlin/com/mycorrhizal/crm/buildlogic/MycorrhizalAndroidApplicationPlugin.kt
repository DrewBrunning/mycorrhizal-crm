package com.mycorrhizal.crm.buildlogic

import com.android.build.api.dsl.ApplicationExtension
import org.gradle.api.Plugin
import org.gradle.api.Project
import org.gradle.kotlin.dsl.configure

/**
 * Convention plugin for the Android application module (:app): applies the
 * android-application + kotlin-android + Compose compiler plugins and the
 * shared Android/JVM/test baseline, plus the Compose build feature flag.
 */
class MycorrhizalAndroidApplicationPlugin : Plugin<Project> {
    override fun apply(target: Project) {
        target.pluginManager.apply("com.android.application")
        target.pluginManager.apply("org.jetbrains.kotlin.android")
        target.pluginManager.apply("org.jetbrains.kotlin.plugin.compose")

        target.extensions.configure<ApplicationExtension>("android") {
            target.configureAndroidCommon(this)
            defaultConfig {
                applicationId = "com.mycorrhizal.crm"
                targetSdk = AndroidConfig.TARGET_SDK
                versionCode = 1
                versionName = "0.1.0"
            }
            buildFeatures {
                compose = true
                buildConfig = true
            }
        }
    }
}
