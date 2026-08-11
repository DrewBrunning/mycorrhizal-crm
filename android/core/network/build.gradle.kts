plugins {
    id("mycorrhizal.android.library")
    id("com.google.devtools.ksp")
}

android {
    namespace = "com.mycorrhizal.crm.network"
}

dependencies {
    api(project(":core:model"))
    api(libs.okhttp)
    implementation(libs.okhttp.logging)
    implementation(libs.moshi)
    implementation(libs.moshi.kotlin)
    implementation(libs.kotlinx.coroutines.core)
    api(libs.kotlinx.datetime)

    testImplementation(libs.mockwebserver)
    testImplementation(libs.kotlinx.coroutines.test)
}
