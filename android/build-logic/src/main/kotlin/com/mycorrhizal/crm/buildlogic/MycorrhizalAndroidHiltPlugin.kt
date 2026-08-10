package com.mycorrhizal.crm.buildlogic

import org.gradle.api.Plugin
import org.gradle.api.Project
import org.gradle.kotlin.dsl.dependencies

/**
 * Convention plugin for Hilt (Dagger): applies the Hilt Gradle plugin and the
 * KSP plugin (Hilt's annotation processor runs through KSP), and wires the
 * hilt-android dependency + compiler.
 */
class MycorrhizalAndroidHiltPlugin : Plugin<Project> {
    override fun apply(target: Project) {
        target.pluginManager.apply("com.google.dagger.hilt.android")
        target.pluginManager.apply("com.google.devtools.ksp")

        target.dependencies {
            "implementation"(target.libs("hilt.android"))
            "ksp"(target.libs("hilt.android.compiler"))
        }
    }
}
