plugins {
    id("mycorrhizal.android.library")
    id("org.jetbrains.kotlin.plugin.compose")
}

android {
    namespace = "com.mycorrhizal.crm.testing"
}

// Issue #214: this module's *main* source set (not `test`) holds shared test
// support code — the Compose semantics-tree a11y assertion helper — so any
// feature module's `test` source set can reach it via
// `testImplementation(project(":core:testing"))`. Kotlin/Gradle don't share
// `test` source sets across modules, so putting it in `main` (the
// Now-In-Android `core:testing` pattern) is the standard way around that.
dependencies {
    implementation(platform(libs.androidx.compose.bom))
    implementation(libs.androidx.compose.ui)
    implementation(libs.androidx.compose.ui.test.junit4)
}
