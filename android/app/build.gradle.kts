plugins {
    id("mycorrhizal.android.application")
    id("mycorrhizal.android.hilt")
}

// M5 §7: release signing via env/properties only — never committed. All four
// variables are required together (a partial set fails fast rather than
// producing an unsigned-with-null-passwords APK at package time); with none
// set the release build stays unsigned (assembleDebug and the CI gate are
// unaffected). Delivery method is decided by whoever wires a CI job to these;
// the keystore itself must never be in the repo.
fun envOrProperty(name: String): String? {
    val value = providers.gradleProperty(name).orNull ?: System.getenv(name)
    return value?.takeIf { it.isNotBlank() }
}

val signingStoreFile = envOrProperty("SIGNING_STORE_FILE")
val signingStorePassword = envOrProperty("SIGNING_STORE_PASSWORD")
val signingKeyAlias = envOrProperty("SIGNING_KEY_ALIAS")
val signingKeyPassword = envOrProperty("SIGNING_KEY_PASSWORD")
val releaseSigningConfigured = listOf(signingStoreFile, signingStorePassword, signingKeyAlias, signingKeyPassword).all { it != null }

android {
    namespace = "com.mycorrhizal.crm"

    signingConfigs {
        if (releaseSigningConfigured) {
            create("release") {
                storeFile = file(signingStoreFile!!)
                storePassword = signingStorePassword
                keyAlias = signingKeyAlias
                keyPassword = signingKeyPassword
            }
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro",
            )
            signingConfig = signingConfigs.findByName("release")
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
    implementation(project(":feature:shares"))
    implementation(project(":feature:audit"))
    implementation(project(":feature:network"))

    // M5 §3.1: Coil's image loader is wired to the authenticated OkHttp stack
    // in MycorrhizalApplication so profile photos load with the bearer JWT.
    implementation(libs.coil.compose)
    implementation(libs.coil.network.okhttp)

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
