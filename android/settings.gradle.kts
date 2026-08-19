pluginManagement {
    repositories {
        google()
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
        mavenCentral()
        // Issue #219/#220: uCrop (the profile-photo crop step) is published
        // only on JitPack — the canonical `com.github.yalantis:ucrop` artifact.
        maven("https://jitpack.io")
    }
}

rootProject.name = "mycorrhizal-android"

include(":app")
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
include(":feature:shares")
include(":feature:audit")
include(":feature:network")
