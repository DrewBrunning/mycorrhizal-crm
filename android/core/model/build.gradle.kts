plugins {
    id("mycorrhizal.android.library")
}

android {
    namespace = "com.mycorrhizal.crm.model"
}

dependencies {
    implementation(libs.moshi)
    implementation(libs.kotlinx.datetime)
    implementation(libs.kotlinx.coroutines.core)
}
