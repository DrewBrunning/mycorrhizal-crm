package com.mycorrhizal.crm.buildlogic

import com.android.build.api.dsl.LibraryExtension
import org.gradle.api.Plugin
import org.gradle.api.Project
import org.gradle.kotlin.dsl.configure

/**
 * Convention plugin for Android library modules: applies the android-library
 * plugin and the shared Android/JVM/test baseline. Kotlin compilation comes
 * from AGP 9 built-in Kotlin.
 */
class MycorrhizalAndroidLibraryPlugin : Plugin<Project> {
    override fun apply(target: Project) {
        target.pluginManager.apply("com.android.library")

        target.extensions.configure<LibraryExtension>("android") {
            target.configureAndroidCommon(this)
        }
    }
}
