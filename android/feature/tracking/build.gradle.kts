plugins {
    id("mycorrhizal.android.library")
    id("mycorrhizal.android.hilt")
    id("org.jetbrains.kotlin.plugin.compose")
}

android {
    namespace = "com.mycorrhizal.crm.feature.tracking"
}

dependencies {
    api(project(":core:data"))
    api(project(":core:network"))
    api(project(":core:domain"))
    implementation(project(":core:ui"))

    implementation(platform(libs.androidx.compose.bom))
    api(libs.androidx.compose.ui)
    implementation(libs.androidx.compose.material.icons.extended)
    implementation(libs.androidx.compose.material3)
    implementation(libs.hilt.android)
    ksp(libs.hilt.android.compiler)
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.work.runtime)
    api(libs.androidx.hilt.work)
    ksp(libs.androidx.hilt.compiler)
    runtimeOnly(libs.kotlinx.coroutines.android)
    // Issue #1133: no Firebase/GMS dependency here. FCM is a distribution
    // variant concern (see the ADR and :app's src/fcm), so this module — and
    // therefore every variant, including the F-Droid FOSS one — compiles with
    // no proprietary push SDK. FcmAvailability/FcmTokenSource are interfaces;
    // the flavor-specific implementations are bound in :app.

    debugImplementation(libs.androidx.compose.ui.tooling)
    debugImplementation(libs.androidx.compose.ui.test.manifest)
    testImplementation(libs.androidx.compose.ui.test.junit4)
    testImplementation(libs.androidx.work.testing)
}
