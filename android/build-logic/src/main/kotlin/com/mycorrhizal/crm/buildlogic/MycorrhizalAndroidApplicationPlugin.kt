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

        // Release CI (docker-publish.yml's build-android-apk job) stamps the real
        // release version via -PMYCORRHIZAL_VERSION_NAME/-PMYCORRHIZAL_VERSION_CODE,
        // derived from the git tag being released, so a GitHub Release's attached
        // APK carries a versionCode Android will actually accept as an upgrade over
        // the previous release (Obtainium auto-update support). Plain Gradle
        // properties, not env vars: unlike the signing secrets these aren't
        // sensitive, so there's no need for the env-var fallback that pattern uses.
        // Every other build (local dev, android-apk-build.yml's manual dispatch)
        // passes neither, and keeps today's static placeholder values.
        val versionNameOverride = target.providers.gradleProperty("MYCORRHIZAL_VERSION_NAME").orNull
        val versionCodeOverride = target.providers.gradleProperty("MYCORRHIZAL_VERSION_CODE").orNull?.toIntOrNull()

        target.extensions.configure<ApplicationExtension>("android") {
            target.configureAndroidCommon(this)
            defaultConfig {
                applicationId = "com.mycorrhizal.crm"
                targetSdk = AndroidConfig.TARGET_SDK
                versionCode = versionCodeOverride ?: 1
                versionName = versionNameOverride ?: "0.1.0"
            }
            buildFeatures {
                compose = true
                buildConfig = true
            }
        }
    }
}
