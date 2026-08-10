plugins {
    id("mycorrhizal.android.library")
    id("org.jetbrains.kotlin.plugin.compose")
}

android {
    namespace = "com.mycorrhizal.crm.ui"
}

dependencies {
    implementation(project(":core:model"))

    implementation(platform(libs.androidx.compose.bom))
    implementation(libs.androidx.compose.ui)
    implementation(libs.androidx.compose.ui.graphics)
    implementation(libs.androidx.compose.ui.tooling.preview)
    implementation(libs.androidx.compose.material3)
    implementation(libs.androidx.compose.material.icons.extended)
    implementation(libs.androidx.compose.foundation)
    implementation(libs.androidx.core.ktx)
    implementation(libs.kotlinx.datetime)

    debugImplementation(libs.androidx.compose.ui.tooling)
}
