pluginManagement {
    repositories {
        google()
        // Issue #2854: the Gradle Plugin Portal redirects plugin jars to
        // Maven Central, where a runner's burst of fresh fetches earns HTTP
        // 429 and the build dies during configuration. Resolve through
        // Google's mirror of Maven Central first (no per-IP throttle);
        // mavenCentral() stays below as the fallback for anything the mirror
        // has not synced yet.
        maven("https://maven-central.storage-download.googleapis.com/maven2/")
        mavenCentral()
        gradlePluginPortal()
    }
}
plugins {
    id("org.gradle.toolchains.foojay-resolver-convention") version "1.0.0"
}

includeBuild("build-logic")

dependencyResolutionManagement {
    repositoriesMode.set(RepositoriesMode.FAIL_ON_PROJECT_REPOS)
    repositories {
        google()
        // Issue #2854: Sonatype answers HTTP 429 to a runner's burst of fresh
        // Maven Central fetches, and the build dies before a single test runs
        // (unit-test jars, the JaCoCo agent, Robolectric's own artifacts). The
        // Google-hosted mirror is a full copy of Maven Central served from
        // Cloud Storage with no per-IP throttle, so resolve through it first
        // and keep mavenCentral() below as the fallback for artifacts the
        // mirror has not synced yet (it syncs a few times a day). Nothing
        // enforces this order; dropping the mirror here silently re-opens
        // #2854.
        maven("https://maven-central.storage-download.googleapis.com/maven2/")
        mavenCentral()
        // Issue #219/#220: uCrop (the profile-photo crop step) is published
        // only on JitPack — the canonical `com.github.yalantis:ucrop` artifact.
        maven("https://jitpack.io")
    }
}

rootProject.name = "mycorrhizal-android"

include(":app")
// Issue #263: startup + dashboard-render macrobenchmark (trend signal, not a
// CI gate). `com.android.test` module — driven by `:macrobenchmark:connectedCheck`.
include(":macrobenchmark")
include(":core:model")
include(":core:network")
include(":core:data")
include(":core:domain")
include(":core:ui")
include(":core:testing")
include(":feature:auth")
include(":feature:contacts")
include(":feature:circles")
include(":feature:tags")
include(":feature:households")
include(":feature:relationships")
include(":feature:timelineentities")
include(":feature:tracking")
include(":feature:import")
include(":feature:timeline")
include(":feature:settings")
include(":feature:cadence")
include(":feature:occasions")
include(":feature:shares")
include(":feature:audit")
include(":feature:trash")
include(":feature:sysevents")
include(":feature:network")
include(":feature:map")
include(":feature:users")
