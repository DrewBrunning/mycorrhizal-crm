package com.mycorrhizal.crm.buildlogic

import com.android.build.api.dsl.LibraryExtension
import org.gradle.api.Plugin
import org.gradle.api.Project
import org.gradle.kotlin.dsl.configure

/**
 * Convention plugin for Android library modules: applies the android-library +
 * kotlin-android plugins and the shared Android/JVM/test baseline.
 */
class MycorrhizalAndroidLibraryPlugin : Plugin<Project> {
    override fun apply(target: Project) {
        target.pluginManager.apply("com.android.library")
        target.pluginManager.apply("org.jetbrains.kotlin.android")

        target.extensions.configure<LibraryExtension>("android") {
            target.configureAndroidCommon(this)
        }
    }
}
