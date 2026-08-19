package com.mycorrhizal.crm.buildlogic

import com.android.build.api.dsl.CommonExtension
import com.android.build.api.dsl.Lint
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
 * compileSdk/minSdk, JVM target, and the common test stack
 * (JUnit 4 + Robolectric + MockK + Turbine + kotlinx-coroutines-test).
 *
 * AGP 9 note: `CommonExtension` is non-generic and exposes only getters for the
 * DSL objects (`defaultConfig`, `compileOptions`, `testOptions`); the `{}` block
 * forms live on the concrete `Application`/`Library` extensions. This shared
 * function mutates through the getter objects so it works on either.
 */
internal object AndroidConfig {
    const val COMPILE_SDK = 35
    const val MIN_SDK = 26
    const val TARGET_SDK = 35
}

/** Applies the Android/JVM/test baseline to any android module. */
internal fun Project.configureAndroidCommon(
    extension: CommonExtension,
) {
    extension.apply {
        compileSdk = AndroidConfig.COMPILE_SDK
        defaultConfig.minSdk = AndroidConfig.MIN_SDK
        compileOptions.sourceCompatibility = JavaVersion.VERSION_17
        compileOptions.targetCompatibility = JavaVersion.VERSION_17
        testOptions.unitTests.isIncludeAndroidResources = true
    }

    extensions.configure<KotlinAndroidProjectExtension>("kotlin") {
        compilerOptions {
            jvmTarget.set(JvmTarget.JVM_17)
        }
    }

    configureAccessibilityLint(extension.lint)
    configureAndroidTestCommon()
}

/**
 * Issue #214: pins every check in Android Lint's built-in `Accessibility`
 * category to `error` so a regression fails `lintDebug`, not just warns.
 * `abortOnError` is left at AGP's default (`true`).
 *
 * This is the full category list, not a hand-picked subset — verified via
 * `lint --show` against lint-checks 32.3.1 (the version bundled with this
 * project's AGP 9.3.1):
 *   - `ContentDescription` — ImageView/ImageButton without a description.
 *   - `LabelFor` — EditText without an associated label.
 *   - `KeyboardInaccessibleWidget` — clickable but not focusable.
 *   - `ClickableViewAccessibility` — a custom onTouchEvent/OnTouchListener
 *     with no matching `performClick`.
 *   - `GetContentDescriptionOverride` — overriding `getContentDescription()`
 *     instead of calling `setContentDescription()`.
 *
 * All five target View/XML layouts. This app is 100% Jetpack Compose and has
 * zero XML layouts today, so this pin is defense-in-depth for any future XML
 * use (a notification layout, a widget, ...), not today's real coverage.
 * Compose's actual a11y surface is the semantics tree, which lint only
 * partially reaches — no bundled lint check (this project's core lint-checks
 * jar, nor the androidx.compose.ui/material/material3/foundation lint jars
 * on the classpath) currently covers touch-target size or Compose semantics
 * misuse. That's what `core:testing`'s `assertAccessibleSemantics` sweep
 * (mounted per top-level screen in each feature module's tests) is for.
 */
internal fun configureAccessibilityLint(lint: Lint) {
    lint.error += setOf(
        "ContentDescription",
        "LabelFor",
        "KeyboardInaccessibleWidget",
        "ClickableViewAccessibility",
        "GetContentDescriptionOverride",
    )
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
