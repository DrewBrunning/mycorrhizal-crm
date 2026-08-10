plugins {
    id("mycorrhizal.android.library")
}

android {
    namespace = "com.mycorrhizal.crm.domain"
}

dependencies {
    api(project(":core:model"))
    implementation(libs.kotlinx.coroutines.core)
    compileOnly("javax.inject:javax.inject:1")
}
