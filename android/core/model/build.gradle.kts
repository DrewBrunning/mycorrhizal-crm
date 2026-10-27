plugins {
    id("mycorrhizal.android.library")
    id("com.google.devtools.ksp")
}

android {
    namespace = "com.mycorrhizal.crm.model"
}

dependencies {
    api(libs.moshi)
    implementation(libs.moshi.kotlin)
    ksp(libs.moshi.kotlin.codegen)
}
