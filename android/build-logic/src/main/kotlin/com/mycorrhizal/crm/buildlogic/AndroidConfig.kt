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
import org.gradle.testing.jacoco.plugins.JacocoTaskExtension
import org.gradle.testing.jacoco.tasks.JacocoReport
import org.gradle.api.file.FileVisitor
import org.gradle.api.file.FileVisitDetails
import org.gradle.api.artifacts.ProjectDependency
import org.jacoco.core.instr.Instrumenter
import org.jacoco.core.runtime.OfflineInstrumentationAccessGenerator
import java.io.File
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

private const val JACOCO_TOOL_VERSION = "0.8.12"

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
    // Room-generated (KSP) DAO/database implementations — like the Hilt/Moshi
    // entries above, generated code nobody hand-writes or reviews, so it's out
    // of scope regardless of whether a test happens to exercise it.
    "**/*_Impl.class",
    "**/*_Impl\$*.class",
)

/**
 * Wires a `jacocoTestReport` task (XML + HTML) off `testDebugUnitTest`'s
 * execution data in every android module.
 *
 * Robolectric's sandboxed classloader loads classes separately from the one
 * JaCoco's on-the-fly agent instruments, so any code exercised only through
 * `RobolectricTestRunner` (every Compose screen, every Room-backed repository)
 * reports 0% despite passing tests. We instrument the compiled classes
 * *offline* (JaCoco `Instrumenter` + `OfflineInstrumentationAccessGenerator`)
 * and prepend the instrumented classes to the unit-test classpath, so
 * Robolectric's sandbox loads the already-instrumented bytes. The on-the-fly
 * agent is disabled and the offline runtime (`org.jacoco.agent:runtime`)
 * records to the same exec file the report already reads.
 */
internal fun Project.configureJacoco() {
    pluginManager.apply("jacoco")

    extensions.configure<JacocoPluginExtension> {
        toolVersion = JACOCO_TOOL_VERSION
    }

    val instrumentedDir = layout.buildDirectory.dir("jacoco/instrumented")
    val instrumentTask = tasks.register("jacocoInstrumentDebug") {
        group = "verification"
        description = "Offline-instruments debug classes for Robolectric coverage."
        val javaClasses = fileTree(layout.buildDirectory.dir("intermediates/javac/debug"))
        val kotlinClasses = fileTree(layout.buildDirectory.dir("tmp/kotlin-classes/debug"))
        dependsOn(tasks.matching { it.name in setOf("compileDebugKotlin", "compileDebugJavaWithJavac") })
        inputs.files(javaClasses, kotlinClasses)
        outputs.dir(instrumentedDir)
        doLast {
            val dest = instrumentedDir.get().asFile
            val instrumenter = Instrumenter(OfflineInstrumentationAccessGenerator())
            val classTree = (javaClasses + kotlinClasses).matching {
                exclude(JACOCO_EXCLUDES)
            }
            classTree.visit(object : FileVisitor {
                override fun visitDir(dirDetails: FileVisitDetails) {}
                override fun visitFile(fileDetails: FileVisitDetails) {
                    if (fileDetails.name.endsWith(".class")) {
                        val target = File(dest, fileDetails.relativePath.pathString)
                        target.parentFile.mkdirs()
                        target.writeBytes(
                            instrumenter.instrument(
                                fileDetails.file.readBytes(),
                                fileDetails.relativePath.pathString,
                            ),
                        )
                    }
                }
            })
        }
    }

    // Offline instrumentation replaces the on-the-fly agent: disable the agent
    // (otherwise plain-JVM classes would be instrumented twice) and point the
    // offline runtime at the same exec file the report reads.
    tasks.withType<Test>().matching { it.name == "testDebugUnitTest" }.configureEach {
        dependsOn(instrumentTask)
        extensions.configure<JacocoTaskExtension> {
            isEnabled = false
        }
        classpath = files(instrumentedDir) + classpath
        systemProperty(
            "jacoco-agent.destfile",
            layout.buildDirectory.file("jacoco/testDebugUnitTest.exec").get().asFile.absolutePath,
        )
        // Issue #342/#357: the exec file is written by the forked test JVM's
        // offline JaCoCo runtime as a side effect, not by a task action, so it
        // was invisible to the build cache. A module whose tests were served
        // FROM-CACHE had its JUnit results restored but no exec file, so the
        // aggregated report merged only the re-run modules' data and Codecov
        // collapsed to ~5%. Declaring it as an (optional) output makes a
        // FROM-CACHE restore bring the exec file back too.
        outputs.file(layout.buildDirectory.file("jacoco/testDebugUnitTest.exec")).optional()
    }

    dependencies {
        "testImplementation"("org.jacoco:org.jacoco.agent:$JACOCO_TOOL_VERSION:runtime")
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

    configureCrossModuleCoverage()
}

/**
 * Wires offline coverage across module boundaries.
 *
 * A module's own `jacocoInstrumentDebug` task only instruments *its* classes,
 * but its Robolectric tests also load the classes of its project dependencies
 * (e.g. `feature:contacts` exercises `core:ui`'s editors and `core:model`'s
 * DTOs). Those dependency classes are loaded un-instrumented, so the coverage
 * is recorded nowhere and `core:ui`/`core:model`/`core:domain` report ~0%
 * despite being exercised by real, passing feature tests.
 *
 * Fix, applied once (guarded by a root extra property) after every project has
 * evaluated:
 *  - prepend every transitive project dependency's instrumented dir to each
 *    module's `testDebugUnitTest` classpath, and
 *  - merge the exec data of every module that (transitively) depends on a
 *    module into that module's `jacocoTestReport`, so the coverage is
 *    attributed to the module that owns the code rather than the module whose
 *    test happened to run it.
 *
 * Reports stay source-scoped (each module's report maps only its own
 * `src/main`), so merging a module's exec into a dependent's report cannot
 * double-count lines — a line is only ever reported by the module that owns it.
 * The only cost is that a single-module `:x:jacocoTestReport` now also runs the
 * tests of every module that depends on `x`.
 *
 * Note: this relies on `gradle.projectsEvaluated` + cross-project task wiring,
 * which is not compatible with Gradle's configuration cache / project
 * isolation. That's acceptable today — this build runs with both disabled —
 * but would need a different mechanism (e.g. an artifact transform) if either
 * is ever turned on.
 */
private fun Project.configureCrossModuleCoverage() {
    val marker = "mycorrhizal.jacoco.crossModuleWired"
    if (rootProject.extensions.extraProperties.has(marker)) return
    rootProject.extensions.extraProperties.set(marker, true)

    gradle.projectsEvaluated {
        val modules = rootProject.subprojects
            .filter { it.tasks.findByName("jacocoInstrumentDebug") != null }

        fun Project.transitiveProjectDeps(): Set<Project> {
            val result = linkedSetOf<Project>()
            val queue = ArrayDeque<Project>().apply { add(this@transitiveProjectDeps) }
            while (queue.isNotEmpty()) {
                val p = queue.removeFirst()
                p.configurations.flatMap { it.allDependencies }
                    .filterIsInstance<ProjectDependency>()
                    .map { rootProject.project(it.path) }
                    .forEach { dep -> if (result.add(dep)) queue.add(dep) }
            }
            result.remove(this@transitiveProjectDeps)
            return result
        }

        val forward = modules.associateWith { it.transitiveProjectDeps() }
        val reverse = modules.associateWith { mod ->
            modules.filter { mod in forward.getValue(it) }.toSet()
        }

        modules.forEach { mod ->
            val testTask = mod.tasks.findByName("testDebugUnitTest") as? Test
            if (testTask != null) {
                val deps = forward.getValue(mod)
                // The module's OWN instrumented classes must be prepended HERE
                // (projectsEvaluated), not in configureJacoco: AGP finalizes the
                // test task's classpath after the convention plugin applies, so
                // the `classpath = files(instrumentedDir) + classpath` made
                // during plugin apply is silently overwritten and the module's
                // own classes run un-instrumented — 0% self coverage while its
                // dependencies (prepended below) still record. Own classes go
                // first so they win over any non-instrumented copy.
                val ownInstrumented = mod.layout.buildDirectory.dir("jacoco/instrumented").get().asFile
                val depDirs = deps.map { it.layout.buildDirectory.dir("jacoco/instrumented").get().asFile }
                testTask.classpath = mod.files(listOf(ownInstrumented) + depDirs) + testTask.classpath
                deps.forEach { dep ->
                    dep.tasks.findByName("jacocoInstrumentDebug")?.let { testTask.dependsOn(it) }
                }
            }

            val report = mod.tasks.findByName("jacocoTestReport") as? JacocoReport
            if (report != null) {
                val contributors = reverse.getValue(mod) + mod
                report.executionData.setFrom(
                    contributors.map { c ->
                        c.fileTree(c.layout.buildDirectory.get().asFile) {
                            include("jacoco/testDebugUnitTest.exec")
                        }
                    },
                )
                contributors.forEach { c ->
                    c.tasks.findByName("testDebugUnitTest")?.let { report.dependsOn(it) }
                }
            }
        }

        // Issue #342: a single aggregated report for Codecov. The per-module
        // `jacocoTestReport` tasks each emit their own XML, and feeding all of
        // them to Codecov made it mis-merge the cross-module exec data — every
        // `feature/*` module reported 0% on the dashboard while the local
        // reports showed ~70%. A single report built from the same exec data
        // (all modules' classes + sources + merged exec) sidesteps Codecov's
        // multi-report merge entirely. Kept separate from the per-module tasks
        // so local HTML browsing per module still works.
        rootProject.pluginManager.apply("jacoco")
        rootProject.tasks.register<JacocoReport>("jacocoTestReportAggregated") {
            group = "verification"
            description = "Merges every module's offline-instrumented exec data into one JaCoCo XML/HTML report."
            dependsOn(modules.mapNotNull { it.tasks.findByName("testDebugUnitTest") })

            reports {
                xml.required.set(true)
                html.required.set(true)
            }

            classDirectories.setFrom(
                modules.flatMap { mod ->
                    listOf(
                        mod.fileTree(mod.layout.buildDirectory.dir("intermediates/javac/debug")) {
                            exclude(JACOCO_EXCLUDES)
                        },
                        mod.fileTree(mod.layout.buildDirectory.dir("tmp/kotlin-classes/debug")) {
                            exclude(JACOCO_EXCLUDES)
                        },
                    )
                },
            )
            sourceDirectories.setFrom(
                modules.flatMap { mod ->
                    listOf(mod.fileTree("src/main/java"), mod.fileTree("src/main/kotlin"))
                },
            )
            executionData.setFrom(
                modules.map { mod ->
                    mod.fileTree(mod.layout.buildDirectory.get()) {
                        include("jacoco/testDebugUnitTest.exec")
                    }
                },
            )
        }
    }
}

internal fun Project.libs(name: String): Any {
    val catalog = extensions.getByType<VersionCatalogsExtension>().named("libs")
    val dependency = catalog.findLibrary(name)
        .orElseThrow { IllegalStateException("No catalog entry '$name'") }
        .get()
    return "${dependency.group}:${dependency.name}:${dependency.version}"
}
