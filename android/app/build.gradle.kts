plugins {
    id("mycorrhizal.android.application")
    id("mycorrhizal.android.hilt")
}

android {
    namespace = "com.mycorrhizal.crm"

    buildTypes {
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro",
            )
        }
    }
}

dependencies {
    implementation(project(":core:data"))
    implementation(project(":core:domain"))
    implementation(project(":core:ui"))
    implementation(project(":feature:auth"))
    implementation(project(":feature:contacts"))
    implementation(project(":feature:circles"))
    implementation(project(":feature:tags"))
    implementation(project(":feature:households"))
    implementation(project(":feature:relationships"))
    implementation(project(":feature:timelineentities"))
    implementation(project(":feature:tracking"))
    implementation(project(":feature:import"))
    implementation(project(":feature:timeline"))
    implementation(project(":feature:settings"))
    implementation(project(":feature:cadence"))
    implementation(project(":feature:audit"))

    implementation(platform(libs.androidx.compose.bom))
    implementation(libs.androidx.compose.ui)
    implementation(libs.androidx.compose.material3)
    implementation(libs.androidx.compose.ui.tooling.preview)
    implementation(libs.androidx.compose.material.icons.extended)
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.work.runtime)
    implementation(libs.androidx.hilt.work)
    ksp(libs.androidx.hilt.compiler)
    implementation(libs.androidx.lifecycle.runtime.compose)
    implementation(libs.androidx.lifecycle.viewmodel.compose)
    implementation(libs.androidx.navigation.compose)
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.core.splashscreen)
    implementation(libs.hilt.android)
    ksp(libs.hilt.android.compiler)
    implementation(libs.hilt.navigation.compose)

    debugImplementation(libs.androidx.compose.ui.tooling)
}
