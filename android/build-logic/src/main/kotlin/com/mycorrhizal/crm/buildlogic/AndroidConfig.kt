package com.mycorrhizal.crm.buildlogic

import com.android.build.api.dsl.CommonExtension
import com.android.build.api.dsl.Lint
import org.gradle.api.JavaVersion
import org.gradle.api.Project
import org.gradle.api.tasks.testing.Test
import org.gradle.api.tasks.testing.logging.TestExceptionFormat
import org.gradle.kotlin.dsl.configure
import org.gradle.kotlin.dsl.dependencies
import org.gradle.kotlin.dsl.register
import org.gradle.kotlin.dsl.withType
import org.gradle.kotlin.dsl.getByType
import org.gradle.testing.jacoco.plugins.JacocoPluginExtension
import org.gradle.testing.jacoco.tasks.JacocoReport
import org.gradle.testretry.TestRetryTaskExtension
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
    // Bumped 35 -> 37 alongside the composeBom update: 2026.08.00's
    // compose-ui/foundation/runtime artifacts require compileSdk 37+.
    // Robolectric unit tests are unaffected -- they pin their own simulated
    // SDK independently (configureAndroidTestCommon's enabledSdks below, and
    // each test class's own `@Config(sdk = [35])`), so this only changes what
    // platform APIs the real app compiles/targets against, not what SDK the
    // test JVM emulates.
    const val COMPILE_SDK = 37
    const val MIN_SDK = 26
    const val TARGET_SDK = 37
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
    configureJacoco()
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
    // Issue #268: retry failed tests once on CI so a flaky test doesn't fail
    // the required check. `failOnPassedAfterRetry` stays false (the plugin
    // default): a test that fails once then passes on retry keeps the task
    // green, and the flake is surfaced by the CI "Detect flaky unit tests"
    // step (Gradle retains the failed attempt in the test-results XML even
    // when the retry passed). Off locally (no CI env var) so dev iteration
    // isn't slowed by re-runs.
    pluginManager.apply("org.gradle.test-retry")
    val isCi = providers.environmentVariable("CI").isPresent
    tasks.withType<Test>().configureEach {
        extensions.configure<TestRetryTaskExtension> {
            if (isCi) {
                maxRetries.set(1)
                maxFailures.set(20)
            }
        }
        testLogging {
            events("passed", "failed", "skipped")
            exceptionFormat = TestExceptionFormat.FULL
        }
        // Robolectric needs the Android resources to resolve themes/drawables.
        systemProperty("robolectric.enabledSdks", "35")
    }
}

// Issue #251: visibility only, no threshold/gate — a separate ticket tracks
// enforcing coverage. Applied to every module (app + library) via
// configureAndroidCommon so `./gradlew jacocoTestReport` at the root reports
// on all of them, the same way bare `./gradlew testDebugUnitTest` already
// does today.
private val JACOCO_EXCLUDES = listOf(
    // AGP/resource-generated.
    "**/R.class",
    "**/R\$*.class",
    "**/BuildConfig.*",
    "**/Manifest*.*",
    // Hilt/Dagger-generated.
    "**/Hilt_*.class",
    "**/*_Hilt*.class",
    "**/*_Factory.class",
    "**/*_MembersInjector.class",
    "**/dagger/hilt/**",
    "**/hilt_aggregated_deps/**",
    // Moshi-generated.
    "**/*JsonAdapter.class",
    // Compose compiler-synthesized holders — not code anyone writes or reviews.
    "**/ComposableSingletons\$*.class",
    "**/*ComposableSingletons*.class",
)

/**
 * Wires a `jacocoTestReport` task (XML + HTML) off `testDebugUnitTest`'s
 * execution data in every android module.
 */
internal fun Project.configureJacoco() {
    pluginManager.apply("jacoco")

    extensions.configure<JacocoPluginExtension> {
        toolVersion = "0.8.12"
    }

    tasks.register<JacocoReport>("jacocoTestReport") {
        group = "verification"
        description = "Generates a code coverage report from testDebugUnitTest."
        dependsOn("testDebugUnitTest")

        reports {
            xml.required.set(true)
            html.required.set(true)
        }

        val javaClasses = fileTree(layout.buildDirectory.dir("intermediates/javac/debug")) {
            exclude(JACOCO_EXCLUDES)
        }
        val kotlinClasses = fileTree(layout.buildDirectory.dir("tmp/kotlin-classes/debug")) {
            exclude(JACOCO_EXCLUDES)
        }
        classDirectories.setFrom(files(javaClasses, kotlinClasses))
        sourceDirectories.setFrom(files("src/main/java", "src/main/kotlin"))
        executionData.setFrom(
            fileTree(layout.buildDirectory.get()) { include("jacoco/testDebugUnitTest.exec") },
        )
    }
}

internal fun Project.libs(name: String): Any {
    val catalog = extensions.getByType<VersionCatalogsExtension>().named("libs")
    val dependency = catalog.findLibrary(name)
        .orElseThrow { IllegalStateException("No catalog entry '$name'") }
        .get()
    return "${dependency.group}:${dependency.name}:${dependency.version}"
}
