plugins {
    id("mycorrhizal.android.library")
    id("com.google.devtools.ksp")
}

android {
    namespace = "com.mycorrhizal.crm.model"
}

dependencies {
    implementation(libs.moshi)
    implementation(libs.moshi.kotlin)
    implementation(libs.kotlinx.datetime)
    implementation(libs.kotlinx.coroutines.core)
    ksp(libs.moshi.kotlin.codegen)
}
