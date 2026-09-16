import org.gradle.api.artifacts.component.ProjectComponentIdentifier

plugins {
    alias(libs.plugins.android.application) apply false
    alias(libs.plugins.android.library) apply false
    alias(libs.plugins.kotlin.android) apply false
    alias(libs.plugins.kotlin.jvm) apply false
    alias(libs.plugins.kotlin.compose) apply false
    alias(libs.plugins.kotlin.serialization) apply false
    alias(libs.plugins.ksp) apply false
    alias(libs.plugins.hilt) apply false
    alias(libs.plugins.google.services) apply false
}

// Issue #942: Gradle dependency locking, so Trivy's license scanner
// (../.github/workflows/license-compliance.yml) has a `gradle.lockfile` per
// module to read licenses from — the same way it reads `go.sum`/`yarn.lock`
// for the other two ecosystems.
//
// Deliberately narrower than the textbook "lock every configuration" recipe
// (`dependencyLocking { lockAllConfigurations() }`): AGP publishes dozens of
// internal `isCanBeResolved` configurations per module (lint, test-fixtures
// metadata, androidTest/unit-test classpaths, the app's benchmark-only build
// type, and — for reasons that don't reproduce through a single named task
// and aren't worth chasing further here — even the plain `debug` compile
// classpath on every module) whose consumer attributes a plain
// `Configuration.resolve()` cannot fully disambiguate the way AGP's own
// variant-aware compile/link tasks do, and fail with a variant-ambiguity
// build error rather than a real dependency problem. `lockAllConfigurations()`
// would also demand a lock entry for every one of those the moment anything
// resolves them (a normal `assembleDebug`/`testDebugUnitTest`), breaking
// ordinary CI runs.
//
// None of that is what ships anyway: `debug`/test dependencies never reach a
// distributed build, and `benchmark` is "[n]ever distributed" per
// app/build.gradle.kts. Only `release` — compile and runtime, the classpath
// that actually becomes the published APK — is what the license/typosquat
// gate this issue is about needs to see, so only that pair is opted into
// locking.
subprojects {
    // `:macrobenchmark` is a `com.android.test` module that is never shipped
    // and has no `release` build type of its own to lock (see its own
    // build.gradle.kts) — nothing here applies to it.
    if (name == "macrobenchmark") {
        return@subprojects
    }

    val lockableClasspaths = setOf(
        "releaseCompileClasspath", "releaseRuntimeClasspath",
    )

    configurations.matching { it.name in lockableClasspaths }.configureEach {
        resolutionStrategy.activateDependencyLocking()
    }

    // Forces real artifact resolution of every external (non-project)
    // dependency on the locked classpaths — the operation both
    // `verifyDependencyLocks` and `downloadLicenseScanArtifacts` need,
    // exposed as a shared, eager (non-lenient) resolution:
    //
    //  - It is what actually enforces the lock: dependency locking rejects a
    //    graph that no longer matches the committed `gradle.lockfile`
    //    ("Cannot find a version ... that satisfies the version
    //    constraints ... Dependency version enforced by Dependency
    //    Locking"), but only when something forces *eager* resolution.
    //    `incoming.resolutionResult` alone is lazy/lenient — it hands back
    //    the graph as data, `FAILED` nodes included, without ever throwing —
    //    so a task that only touches `resolutionResult` silently reports
    //    success even when a manifest change has drifted from the lock. A
    //    real artifact fetch does throw.
    //  - Excluding project components from the `ArtifactView` (rather than
    //    passing `lenient(true)`, which would also swallow the real lock
    //    failure above) is what avoids the secondary-variant ambiguity
    //    described below — local modules were never a license or
    //    lock-drift question in the first place.
    //
    // Plain `Configuration.resolve()` would hit that ambiguity: AGP's
    // project-to-project configurations (`releaseApiElements`/
    // `releaseRuntimeElements`) publish several secondary variants
    // (android-manifest, r-class-jar, jar, android-lint, …) distinguished
    // only by the `artifactType` attribute that a generic resolve doesn't
    // specify — AGP's own compile/link tasks request a specific one via
    // their own ArtifactView, but a bare `.resolve()` here cannot, and fails
    // with a variant-ambiguity error against every local module dependency,
    // not a real dependency problem.
    fun resolveExternalArtifacts() {
        configurations
            .filter { it.isCanBeResolved && it.name in lockableClasspaths }
            .forEach { config ->
                config.incoming.artifactView {
                    componentFilter { id -> id !is ProjectComponentIdentifier }
                }.files.files
            }
    }

    tasks.register("resolveAndLockAll") {
        notCompatibleWithConfigurationCache("Filters configurations at execution time")
        doFirst {
            require(gradle.startParameter.isWriteDependencyLocks) {
                "resolveAndLockAll must be run as `./gradlew resolveAndLockAll --write-locks`"
            }
        }
        doLast { resolveExternalArtifacts() }
    }

    // The verify-lockfile CI step (issue #942's fix): the same eager
    // resolution as above, without `--write-locks`, so a manifest change
    // that has drifted from the committed lock fails the build instead of
    // silently regenerating it.
    tasks.register("verifyDependencyLocks") {
        notCompatibleWithConfigurationCache("Filters configurations at execution time")
        doLast { resolveExternalArtifacts() }
    }

    // license-compliance.yml's Gradle job companion to `go mod download` /
    // `yarn install`: Trivy's license scanner reads LICENSE metadata out of
    // the actual downloaded artifact files, not the lockfile's coordinates
    // alone, so those files must exist in the local Gradle module cache
    // before Trivy runs.
    tasks.register("downloadLicenseScanArtifacts") {
        notCompatibleWithConfigurationCache("Filters configurations at execution time")
        doLast { resolveExternalArtifacts() }
    }
}
