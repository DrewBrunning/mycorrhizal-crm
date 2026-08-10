package com.mycorrhizal.crm.buildlogic

import com.android.build.api.dsl.CommonExtension
import org.gradle.api.JavaVersion
import org.gradle.api.Project
import org.gradle.api.tasks.testing.Test
import org.gradle.api.tasks.testing.logging.TestExceptionFormat
import org.gradle.kotlin.dsl.configure
import org.gradle.kotlin.dsl.dependencies
import org.gradle.kotlin.dsl.withType
import org.gradle.kotlin.dsl.getByType
import org.jetbrains.kotlin.gradle.dsl.JvmTarget
import org.jetbrains.kotlin.gradle.dsl.KotlinAndroidProjectExtension
import org.gradle.api.artifacts.VersionCatalogsExtension

/**
 * Shared Android configuration for both library and application modules:
 * compileSdk/minSdk/targetSdk, JVM target, and the common test stack
 * (JUnit 4 + Robolectric + MockK + Turbine + kotlinx-coroutines-test).
 */
internal object AndroidConfig {
    const val COMPILE_SDK = 35
    const val MIN_SDK = 26
    const val TARGET_SDK = 35
    const val JDK_TARGET = "17"
}

/** Applies the Android/JVM/test baseline to any android module. */
internal fun Project.configureAndroidCommon(
    extension: CommonExtension<*, *, *, *, *, *>,
) {
    extension.apply {
        compileSdk = AndroidConfig.COMPILE_SDK
        defaultConfig {
            minSdk = AndroidConfig.MIN_SDK
        }
        compileOptions {
            sourceCompatibility = JavaVersion.VERSION_17
            targetCompatibility = JavaVersion.VERSION_17
        }
        testOptions {
            unitTests.isIncludeAndroidResources = true
        }
    }

    extensions.configure<KotlinAndroidProjectExtension>("kotlin") {
        compilerOptions {
            jvmTarget.set(JvmTarget.JVM_17)
        }
    }

    configureAndroidTestCommon()
}

internal fun Project.configureAndroidTestCommon() {
    dependencies {
        "testImplementation"(libs("junit"))
        "testImplementation"(libs("robolectric"))
        "testImplementation"(libs("androidx.test.core"))
        "testImplementation"(libs("androidx.test.ext.junit"))
        "testImplementation"(libs("mockk"))
        "testImplementation"(libs("turbine"))
        "testImplementation"(libs("kotlinx.coroutines.test"))
    }
    tasks.withType<Test>().configureEach {
        testLogging {
            events("passed", "failed", "skipped")
            exceptionFormat = TestExceptionFormat.FULL
        }
        // Robolectric needs the Android resources to resolve themes/drawables.
        systemProperty("robolectric.enabledSdks", "35")
    }
}

internal fun Project.libs(name: String): Any {
    val catalog = extensions.getByType<VersionCatalogsExtension>().named("libs")
    val dependency = catalog.findLibrary(name)
        .orElseThrow { IllegalStateException("No catalog entry '$name'") }
        .get()
    return "${dependency.group}:${dependency.name}:${dependency.version}"
}
