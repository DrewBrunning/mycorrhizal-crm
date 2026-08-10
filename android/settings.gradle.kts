pluginManagement {
    repositories {
        google()
        mavenCentral()
        gradlePluginPortal()
    }
}

includeBuild("build-logic")

dependencyResolutionManagement {
    repositoriesMode.set(RepositoriesMode.FAIL_ON_PROJECT_REPOS)
    repositories {
        google()
        mavenCentral()
    }
}

rootProject.name = "mycorrhizal-android"

include(":app")
include(":core:model")
include(":core:network")
include(":core:data")
include(":core:domain")
include(":core:ui")
include(":feature:auth")
include(":feature:contacts")
include(":feature:timeline")
