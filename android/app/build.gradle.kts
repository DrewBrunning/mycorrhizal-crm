plugins {
    id("mycorrhizal.android.application")
    id("mycorrhizal.android.hilt")
}

// M5 §5a (issue #152): the Google Services Gradle plugin is applied ONLY when
// a real `google-services.json` exists in this module. That file is an external
// resource (a per-deploy Firebase project — one is never committed), so without
// it the app must still build: the Firebase SDK compiles fine and simply finds
// no configured FirebaseApp at runtime, which flips the FCM path to the
// polling-worker fallback (see feature:tracking's FcmAvailability). The plugin
// must be applied before the `android {}` block, hence this top-of-file apply.
if (file("google-services.json").exists()) {
    apply(plugin = "com.google.gms.google-services")
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

    defaultConfig {
        // Issue #238: instrumented end-to-end tests (app/src/androidTest) drive
        // the real app against the docker-compose.test.yml backend on an
        // emulator/device via `./gradlew connectedDebugAndroidTest`.
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }

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
    implementation(project(":feature:users"))

    // M5 §3.1: Coil's image loader is wired to the authenticated OkHttp stack
    // in MycorrhizalApplication so profile photos load with the bearer JWT.
    implementation(libs.coil.compose)
    implementation(libs.coil.network.okhttp)

    implementation(platform(libs.androidx.compose.bom))
    // Issue #238: version alignment for Gradle 9's consistent resolution — see
    // the androidx-concurrent-futures catalog entry.
    implementation(libs.androidx.concurrent.futures)
    implementation(libs.androidx.compose.ui)
    implementation(libs.androidx.compose.material3)
    implementation(libs.androidx.compose.material3.window.size)
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
    debugImplementation(libs.androidx.compose.ui.test.manifest)
    testImplementation(libs.androidx.compose.ui.test.junit4)

    // Issue #238: instrumented E2E tests (app/src/androidTest) — Compose UI
    // test + the AndroidJUnitRunner against the real docker-compose.test.yml
    // backend. The app's own implementation deps (Hilt, OkHttp, Moshi, ...) are
    // already on the androidTest runtime classpath, so the seeding helper can
    // reuse them for its API calls.
    androidTestImplementation(libs.junit)
    androidTestImplementation(libs.androidx.test.core)
    androidTestImplementation(libs.androidx.test.runner)
    androidTestImplementation(libs.androidx.test.ext.junit)
    // Explicit override of compose ui-test's transitive espresso 3.5.0: the
    // compose Android test environment syncs through Espresso.onIdle() on
    // device, and espresso < 3.7.0 uses InputManager.getInstance(), which was
    // removed on API 36+ (fails on the API 37 CI emulator).
    androidTestImplementation(libs.androidx.test.espresso.core)
    androidTestImplementation(libs.androidx.compose.ui.test.junit4)
    androidTestImplementation(libs.androidx.compose.ui.test.manifest)
}
