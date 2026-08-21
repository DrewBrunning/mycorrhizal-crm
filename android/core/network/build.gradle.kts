plugins {
    id("mycorrhizal.android.library")
    id("com.google.devtools.ksp")
}

android {
    namespace = "com.mycorrhizal.crm.network"

    // Issue #257: the checked-in contract fixtures at /testdata/contract-fixtures
    // (repo root) are the single canonical copy web's vitest suite reads via a
    // TS `import`; this adds them to :core:network's test classpath directly
    // rather than duplicating the files into src/test/resources.
    sourceSets {
        getByName("test") {
            resources.srcDir("../../../testdata/contract-fixtures")
        }
    }
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
