plugins {
    id("mycorrhizal.android.library")
    id("org.jetbrains.kotlin.plugin.compose")
}

android {
    namespace = "com.mycorrhizal.crm.ui"
}

dependencies {
    api(project(":core:model"))
    api(project(":core:domain"))
    api(project(":core:network"))

    implementation(libs.androidx.activity.compose)
    implementation(platform(libs.androidx.compose.bom))
    api(libs.androidx.compose.ui)
    implementation(libs.androidx.compose.ui.graphics)
    api(libs.androidx.compose.material3)
    implementation(libs.androidx.compose.material.icons.extended)
    api(libs.androidx.compose.foundation)

    debugImplementation(libs.androidx.compose.ui.tooling)
    debugImplementation(libs.androidx.compose.ui.test.manifest)
    testImplementation(libs.androidx.compose.ui.test.junit4)
}

// NoRawErrorMessageTest reads the feature modules' sources; declare them so a
// change there re-runs the test instead of reusing a cached pass.
tasks.withType<Test>().configureEach {
    inputs.files(fileTree("../../feature") { include("*/src/main/**/*.kt") })
        .withPropertyName("featureSources")
        .withPathSensitivity(PathSensitivity.RELATIVE)
}
