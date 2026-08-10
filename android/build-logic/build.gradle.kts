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
    implementation("com.android.tools.build:gradle:8.7.3")
    implementation("org.jetbrains.kotlin:kotlin-gradle-plugin:2.1.0")
    implementation("org.jetbrains.kotlin.plugin.compose:org.jetbrains.kotlin.plugin.compose.gradle.plugin:2.1.0")
    implementation("com.google.devtools.ksp:com.google.devtools.ksp.gradle.plugin:2.1.0-1.0.29")
    implementation("com.google.dagger:hilt-android-gradle-plugin:2.51.1")
}
