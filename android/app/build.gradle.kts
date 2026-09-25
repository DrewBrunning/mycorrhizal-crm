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
//
// Issue #1133: this plugin has no per-flavor switch, so it is deliberately
// never applied in F-Droid's checkout (which ships no google-services.json).
// A developer who does have one present gets it processed for every flavor,
// including `foss` — that is a local convenience, not a shipped build; the
// FOSS APK is built by F-Droid without the file and therefore without the
// plugin, and it carries no Firebase dependency either way.
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
        // emulator/device via `./gradlew :app:connectedObtainiumDebugAndroidTest`.
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }

    // Issue #1133: three distribution flavors, one per release channel. The
    // dimension is declared on :app only — the library modules stay
    // flavor-free, and all proprietary push code lives in the `obtainium`/`play`
    // source sets here. See docs/adrs/0022-distribution-variants.md.
    //
    //   obtainium — the gold-standard self-hosted build shipped as a GitHub
    //               Release APK and updated by Obtainium. FCM present (optional
    //               at runtime; needs an external google-services.json).
    //   play      — Google Play build. FCM present, same feature set as
    //               obtainium today; its own source set so Play-only additions
    //               (in-app updates, Play Integrity) have somewhere to land.
    //   foss      — F-Droid build. No Firebase/GMS; reminder push comes solely
    //               from the WorkManager polling workers. This is the only
    //               variant F-Droid's build server is asked to build.
    flavorDimensions += "distribution"
    productFlavors {
        create("obtainium") { dimension = "distribution" }
        create("play") { dimension = "distribution" }
        create("foss") { dimension = "distribution" }
    }

    sourceSets {
        // The Firebase-backed push implementation is shared by obtainium and
        // play only (src/foss binds a no-op instead). Sharing the directory
        // keeps one copy of MyFirebaseMessagingService/FirebaseFcmTokenSource
        // rather than two identical flavor trees.
        //
        // Only Kotlin is remapped, deliberately: overriding the source set's
        // `manifest.srcFile` would replace its default `src/<flavor>/AndroidManifest.xml`,
        // and both obtainium and play need to keep their own flavor manifest
        // (play's is the issue #1200 permission carve-out). The FCM service is
        // declared in the main manifest and removed by `src/foss` instead.
        getByName("obtainium").kotlin.srcDir("src/fcm/kotlin")
        getByName("play").kotlin.srcDir("src/fcm/kotlin")
        // The FCM service test compiles against RemoteMessage, so it exists
        // only for the two FCM flavors.
        getByName("testObtainium").kotlin.srcDir("src/fcmTest/kotlin")
        getByName("testPlay").kotlin.srcDir("src/fcmTest/kotlin")
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

        // Issue #263: the build type the `:macrobenchmark` module measures. It
        // inherits release's R8 + resource-shrinking config so the numbers
        // reflect a shipping-shaped build, but:
        //   - debug-signed, so it installs on the CI emulator without the
        //     release keystore (which is env-only and absent in the Android
        //     jobs — see the signingConfigs block above);
        //   - non-debuggable + `isProfileable`, so macrobenchmark can read
        //     startup and frame timing (a debuggable build is rejected /
        //     warns, and JIT/debug overhead pollutes the numbers).
        // Never distributed — no CI job assembles it except the macrobenchmark
        // one, and the GitHub Release APK is still the `release` variant.
        create("benchmark") {
            initWith(getByName("release"))
            signingConfig = signingConfigs.getByName("debug")
            matchingFallbacks += listOf("release")
            isDebuggable = false
            isProfileable = true
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
    implementation(project(":feature:occasions"))
    implementation(project(":feature:shares"))
    implementation(project(":feature:audit"))
    implementation(project(":feature:sysevents"))
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
    // Issue #722: the app-lock gate shows a BiometricPrompt, which needs a
    // FragmentActivity host (MainActivity) — fragment-ktx is the only fragment
    // use in this single-Activity Compose app.
    implementation(libs.androidx.biometric)
    implementation(libs.androidx.fragment.ktx)
    implementation(libs.hilt.android)
    ksp(libs.hilt.android.compiler)
    implementation(libs.hilt.navigation.compose)

    debugImplementation(libs.androidx.compose.ui.tooling)
    debugImplementation(libs.androidx.compose.ui.test.manifest)
    testImplementation(libs.androidx.compose.ui.test.junit4)
    // Issue #679: back-stack / route-table assertions against a real
    // TestNavHostController in Robolectric.
    testImplementation(libs.androidx.navigation.testing)

    // Issue #238: instrumented E2E tests (app/src/androidTest) — Compose UI
    // test + the AndroidJUnitRunner against the real docker-compose.test.yml
    // backend. The app's own implementation deps (Hilt, OkHttp, Moshi, ...) are
    // already on the androidTest runtime classpath, so the seeding helper can
    // reuse them for its API calls.
    androidTestImplementation(libs.junit)
    androidTestImplementation(libs.androidx.test.core)
    androidTestImplementation(libs.androidx.test.runner)
    androidTestImplementation(libs.androidx.test.ext.junit)
    // Issue #385: the storage-layer instrumented tests (RoomCacheEncryptionTest)
    // build Room + SQLCipher databases directly; `core:data`'s implementation
    // deps are runtime-only, so Room/SQLCipher are declared here for the
    // androidTest compile classpath.
    androidTestImplementation(libs.room.runtime)
    androidTestImplementation(libs.sqlcipher.android)
    // Explicit override of compose ui-test's transitive espresso 3.5.0: the
    // compose Android test environment syncs through Espresso.onIdle() on
    // device, and espresso < 3.7.0 uses InputManager.getInstance(), which was
    // removed on API 36+ — no CI emulator reaches that level today
    // (android-tests.yml pins API 24/26/35), but a real device/emulator at
    // 36+ does, hence the explicit override.
    androidTestImplementation(libs.androidx.test.espresso.core)
    androidTestImplementation(libs.androidx.compose.ui.test.junit4)
    androidTestImplementation(libs.androidx.compose.ui.test.manifest)
    // Issue #914: ServerTooOldGateE2ETest stubs a below-baseline /health
    // response — no real server is below this app's own migration floor to
    // boot for that case (see the test's doc comment).
    androidTestImplementation(libs.mockwebserver)

    // Issue #1133: the proprietary push SDK, obtainium and play only. The
    // foss (F-Droid) flavor has neither dependency, so no Firebase/GMS class
    // can end up in that APK (F-Droid's inclusion policy forbids them).
    // kotlinx-coroutines-play-services is only needed to bridge the FCM token
    // Task into a suspend fun in FirebaseFcmTokenSource.
    "obtainiumImplementation"(libs.firebase.messaging)
    "obtainiumImplementation"(libs.kotlinx.coroutines.play.services)
    "playImplementation"(libs.firebase.messaging)
    "playImplementation"(libs.kotlinx.coroutines.play.services)
}
