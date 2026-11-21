plugins {
    id("mycorrhizal.android.library")
    id("mycorrhizal.android.hilt")
    id("org.jetbrains.kotlin.plugin.compose")
    // Issue #2534: record/verify Compose screenshot goldens on the Robolectric
    // stack (recordRoborazziDebug / verifyRoborazziDebug).
    alias(libs.plugins.roborazzi)
}

// Issue #2534: goldens are committed next to the tests, not in build/, so a
// review sees the changed pixels. A failing verify writes
// `<golden>_actual`/`<golden>_compare` under build/outputs/roborazzi/, which the
// android-unit CI job uploads on failure.
roborazzi {
    outputDir.set(file("src/test/screenshots"))
}

android {
    namespace = "com.mycorrhizal.crm.feature.auth"
}

dependencies {
    api(project(":core:data"))
    api(project(":core:network"))
    api(project(":core:domain"))
    implementation(project(":core:ui"))

    implementation(platform(libs.androidx.compose.bom))
    api(libs.androidx.compose.ui)
    implementation(libs.androidx.compose.material3)
    implementation(libs.androidx.lifecycle.runtime.compose)
    implementation(libs.androidx.lifecycle.viewmodel.compose)
    implementation(libs.hilt.android)
    ksp(libs.hilt.android.compiler)

    debugImplementation(libs.androidx.compose.ui.tooling)
    debugImplementation(libs.androidx.compose.ui.test.manifest)
    testImplementation(libs.androidx.compose.ui.test.junit4)
    // Issue #2534: Compose screenshot regression. The capture extensions we use
    // (SemanticsNodeInteraction.captureRoboImage + RobolectricDeviceQualifiers)
    // ship in `roborazzi` itself, so only that artifact is on the module's test
    // classpath; see :feature:contacts for the rule-free API that is not used.
    testImplementation(libs.roborazzi)
    testImplementation(project(":core:testing"))
}
