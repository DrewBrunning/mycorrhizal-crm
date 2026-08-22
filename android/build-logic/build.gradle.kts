plugins {
    `kotlin-dsl`
}

gradlePlugin {
    plugins {
        register("mycorrhizal.android.application") {
            id = "mycorrhizal.android.application"
            implementationClass = "com.mycorrhizal.crm.buildlogic.MycorrhizalAndroidApplicationPlugin"
        }
        register("mycorrhizal.android.library") {
            id = "mycorrhizal.android.library"
            implementationClass = "com.mycorrhizal.crm.buildlogic.MycorrhizalAndroidLibraryPlugin"
        }
        register("mycorrhizal.android.hilt") {
            id = "mycorrhizal.android.hilt"
            implementationClass = "com.mycorrhizal.crm.buildlogic.MycorrhizalAndroidHiltPlugin"
        }
    }
}

dependencies {
    implementation("com.android.tools.build:gradle:9.3.1")
    implementation("org.jetbrains.kotlin:kotlin-gradle-plugin:2.3.10")
    implementation("org.jetbrains.kotlin.plugin.compose:org.jetbrains.kotlin.plugin.compose.gradle.plugin:2.3.10")
    implementation("com.google.devtools.ksp:com.google.devtools.ksp.gradle.plugin:2.3.11")
    implementation("com.google.dagger:hilt-android-gradle-plugin:2.60.1")
    // Issue #358: detekt applied to every module from the convention plugins
    // (see MycorrhizalAndroid{Application,Library}Plugin); version kept in
    // sync with the catalog [versions].detekt / [plugins].detekt alias.
    implementation("io.gitlab.arturbosch.detekt:io.gitlab.arturbosch.detekt.gradle.plugin:1.23.8")
    // Issue #268: org.gradle.test-retry (retries failed Robolectric unit tests
    // in CI so a flake doesn't fail the required check). On the convention
    // plugin classpath so AndroidConfig can apply it to every module.
    implementation(libs.gradle.test.retry.plugin)
    // JaCoCo offline instrumentation (Robolectric sandbox coverage): the core
    // Instrumenter runs inside the convention plugin's custom instrument task.
    implementation("org.jacoco:org.jacoco.core:0.8.12")
}
