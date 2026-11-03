pluginManagement {
    includeBuild("../build-logic")
}

dependencyResolutionManagement {
    // Issue #2307: the `kotlin-dsl` plugin registers the Gradle Plugin Portal
    // as a project repository, and the default PREFER_PROJECT mode made it the
    // only repository for this build's `classpath` configuration -- so every
    // Kotlin/AGP/detekt artifact came from plugins.gradle.org even though the
    // repositories below list Maven Central. A portal hiccup then failed the
    // Android E2E (minSdk 26) job with "Could not find kotlin-stdlib-2.3.20.jar"
    // while sibling jobs with warm caches passed, and the E2E retry reused the
    // same negative cache. PREFER_SETTINGS keeps these repositories in charge
    // so Maven Central can serve a jar the portal is temporarily missing.
    repositoriesMode.set(RepositoriesMode.PREFER_SETTINGS)
    repositories {
        google()
        mavenCentral()
        // Issue #268: the org.gradle.test-retry plugin is published only to
        // the Gradle Plugin Portal (not mavenCentral); build-logic needs it
        // on its classpath to apply it from the shared convention config.
        // Fallback only (#2307): keep it last so Maven Central is preferred.
        gradlePluginPortal()
    }
    versionCatalogs {
        create("libs") {
            from(files("../gradle/libs.versions.toml"))
        }
    }
}

rootProject.name = "mycorrhizal-build-logic"
