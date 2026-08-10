plugins {
    id("mycorrhizal.android.library")
    id("mycorrhizal.android.hilt")
    id("com.google.devtools.ksp")
}

android {
    namespace = "com.mycorrhizal.crm.data"
}

dependencies {
    api(project(":core:domain"))
    api(project(":core:network"))
    implementation(project(":core:model"))

    implementation(libs.hilt.android)
    ksp(libs.hilt.android.compiler)

    implementation(libs.room.runtime)
    implementation(libs.room.ktx)
    ksp(libs.room.compiler)

    implementation(libs.androidx.datastore.preferences)
    implementation(libs.androidx.security.crypto)
    implementation(libs.kotlinx.coroutines.android)
    implementation(libs.moshi)
    implementation(libs.moshi.kotlin)
    ksp(libs.moshi.kotlin.codegen)

    testImplementation(libs.room.testing)
    testImplementation(libs.kotlinx.coroutines.test)
}
