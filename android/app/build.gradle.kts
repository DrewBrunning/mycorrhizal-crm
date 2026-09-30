import org.gradle.api.tasks.Exec

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

// ADR 0028 Decision 2: where the packaged embedded server binary is written,
// before jniLibs packaging picks it up (see the sourceSets entry and the
// buildEmbeddedServer task below).
val embeddedServerJniLibs = layout.buildDirectory.dir("generated/embeddedServer/jniLibs")

android {
    namespace = "com.mycorrhizal.crm"

    defaultConfig {
        // Issue #238: instrumented end-to-end tests (app/src/androidTest) drive
        // the real app against the docker-compose.test.yml backend on an
        // emulator/device via `./gradlew :app:connectedObtainiumDebugAndroidTest`.
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
        // ADR 0028 Decision 1/2: the "Use on this device only" (Local profile)
        // entry was held off until the embedded backend host (issue #1262) AND
        // bundle export/backup (issue #1264) both shipped — a local profile holds
        // the only copy of its data. Both have, so it is on (issue #1108).
        buildConfigField("boolean", "LOCAL_MODE_ENABLED", "true")
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

        // Issue #1268 / ADR 0029 §5: the shared deep-link route vectors at
        // /testdata/deep-links (repo root) — one hand-authored file every
        // parser (Android, web, backend emitter) is tested against.
        getByName("test").resources.srcDir("../../testdata/deep-links")

        // ADR 0028 Decision 2: the generated arm64-v8a server binary (see the
        // buildEmbeddedServer task below) is merged into the APK's jniLibs
        // exactly like a checked-in native library. The directory is empty (and
        // the file optional) until the task runs, so an ordinary build without
        // the Go toolchain still succeeds — it just ships without local mode.
        getByName("main").jniLibs.srcDir(embeddedServerJniLibs.get().asFile)
    }

    // ADR 0028 Decision 2: the embedded Go server ships as an executable in
    // jniLibs/arm64-v8a, run from applicationInfo.nativeLibraryDir. Executing
    // an extracted native binary requires legacy packaging (native libraries
    // stored compressed and extracted at install time). This also compresses
    // the SQLCipher libraries already in the APK, which is why the net size
    // growth is smaller than the server's own compressed size.
    packaging {
        jniLibs {
            useLegacyPackaging = true
        }
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
    // ADR 0028 Decision 2: ProcessLifecycleOwner stops the embedded local
    // server when the app is backgrounded.
    implementation(libs.androidx.lifecycle.process)
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
    // Issue #1293 / ADR 0034: the Credential Manager *provider* for Android < 14
    // (Google Password Manager via Play services). Runtime-only — androidx.credentials
    // discovers it reflectively — and deliberately absent from `foss`: F-Droid forbids
    // GMS, so on that flavor passkeys work only where the platform ships Credential
    // Manager (Android 14+) and otherwise degrade under the same gate.
    // Guarded by the FOSS Firebase/GMS-free APK check and FossFlavorGmsFreeTest.
    "obtainiumRuntimeOnly"(libs.androidx.credentials.play.services.auth)
    "playRuntimeOnly"(libs.androidx.credentials.play.services.auth)
}

// ---------------------------------------------------------------------------
// ADR 0028 Decision 2 / issue #1262: the embedded Go server binary.
//
// The backend is cross-compiled for `GOOS=android GOARCH=arm64 CGO_ENABLED=0
// -tags nodynamic` and dropped into jniLibs/arm64-v8a as `libmycorrhizal.so`
// (the executable name pattern Android extracts from nativeLibraryDir). It is
// BUILT, never committed: CI/F-Droid build it from the same commit as the app,
// and this task is the one build step that does so.
//
// `-tags nodynamic` is required: GOOS=android satisfies the `linux` build
// constraint, so heic's optional libheif dlopen path (via purego) would be
// compiled in and needs cgo on Android. The tag keeps the pure-WASM decoder.
//
// Only arm64-v8a is built or shipped (ADR 0028, "arm64-v8a only"): GOOS=android
// links internally only for arm64, and the pure-Go SQLite stack's x86_64 legacy
// syscalls are blocked by Android's seccomp filter. The app hides the Local
// profile on any other ABI (LocalServerAvailability).
//
// The task is skipped when the Go toolchain is absent, so an ordinary Gradle
// build without Go still succeeds (it just ships no local mode). Set the
// MYCORRHIZAL_BUILD_EMBEDDED_SERVER=true property to make the packaging tasks
// build it automatically.
val goAvailable: Boolean = runCatching {
    val probe = ProcessBuilder("go", "version").redirectErrorStream(true).start()
    probe.waitFor()
    probe.exitValue() == 0
}.getOrDefault(false)

val embeddedServerOutput = embeddedServerJniLibs.map { it.file("arm64-v8a/libmycorrhizal.so").asFile }

val buildEmbeddedServer by tasks.registering(Exec::class) {
    group = "build"
    description = "Cross-compiles the embedded Go server for Android arm64-v8a into jniLibs."
    workingDir = rootProject.file("../backend")
    environment("CGO_ENABLED", "0")
    environment("GOOS", "android")
    environment("GOARCH", "arm64")
    // backend/go.mod pins a Go toolchain newer than some runners' preinstalled
    // `go`. auto lets whatever `go` is on PATH fetch the required toolchain
    // instead of failing with "go.mod requires go >= 1.26.0 ... GOTOOLCHAIN=local".
    environment("GOTOOLCHAIN", "auto")
    commandLine(
        "go", "build",
        "-tags", "nodynamic",
        "-trimpath",
        "-buildvcs=false",
        "-o", embeddedServerOutput.get().absolutePath,
        ".",
    )
    onlyIf { goAvailable }
    doFirst { embeddedServerOutput.get().parentFile.mkdirs() }
}

if (providers.gradleProperty("MYCORRHIZAL_BUILD_EMBEDDED_SERVER").orNull == "true") {
    tasks.named("preBuild").configure { dependsOn(buildEmbeddedServer) }
}

// Issue #1357 (ADR 0029): the merged manifest of every shipped flavor x {debug, release} is
// handed to ExportedSurfaceTest, which compares the externally-invokable surface (exported
// components + every intent-filter, app code AND library contributions) with
// exported-surface-allowlist.txt. The unit-test tasks depend on the merged manifests, so the
// check runs in the existing `testObtainiumDebugUnitTest` / `testFossDebugUnitTest` PR steps.
val mergedManifestsForSurfaceCheck = mutableMapOf<String, Provider<RegularFile>>()
androidComponents {
    onVariants { variant ->
        val type = variant.buildType
        if (type == "debug" || type == "release") {
            mergedManifestsForSurfaceCheck[variant.name] =
                variant.artifacts.get(com.android.build.api.artifact.SingleArtifact.MERGED_MANIFEST)
        }
    }
}
val exportedSurfaceAllowlist = layout.projectDirectory.file("exported-surface-allowlist.txt")
tasks.withType<Test>().configureEach {
    inputs.file(exportedSurfaceAllowlist)
    inputs.files(provider { mergedManifestsForSurfaceCheck.values.toList() })
    jvmArgumentProviders += CommandLineArgumentProvider {
        listOf(
            "-DexportedSurface.allowlist=${exportedSurfaceAllowlist.asFile.absolutePath}",
            "-DexportedSurface.manifests=" +
                mergedManifestsForSurfaceCheck.entries.sortedBy { it.key }
                    .joinToString(";") { "${it.key}=${it.value.get().asFile.absolutePath}" },
        )
    }
}
