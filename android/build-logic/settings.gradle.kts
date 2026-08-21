pluginManagement {
    includeBuild("../build-logic")
}

dependencyResolutionManagement {
    repositories {
        google()
        mavenCentral()
        // Issue #268: the org.gradle.test-retry plugin is published only to
        // the Gradle Plugin Portal (not mavenCentral); build-logic needs it
        // on its classpath to apply it from the shared convention config.
        gradlePluginPortal()
    }
    versionCatalogs {
        create("libs") {
            from(files("../gradle/libs.versions.toml"))
        }
    }
}

rootProject.name = "mycorrhizal-build-logic"
