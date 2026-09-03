import org.jetbrains.kotlin.gradle.dsl.JvmTarget

/**
 * Issue #263: `:macrobenchmark` — an `androidx.benchmark.macro` module measuring
 * app cold/warm/hot startup ([StartupBenchmark]) and dashboard render/scroll
 * frame timing ([DashboardRenderBenchmark]).
 *
 * This is deliberately a bare `com.android.test` module: it does NOT apply the
 * `mycorrhizal.android.*` convention plugins (no detekt / JaCoCo / Robolectric
 * stack — none of which make sense for an on-device benchmark), and it does NOT
 * apply the `androidx.benchmark` Gradle plugin (only the runtime library), so
 * there is no AGP-version compatibility gate to keep in sync.
 *
 * Run it (needs an emulator/device — see README-developer.md → "Android
 * macrobenchmark"):
 *
 *   cd android && ./gradlew :macrobenchmark:connectedCheck \
 *     -Pandroidx.benchmark.suppressErrors=EMULATOR,UNLOCKED
 *
 * The dashboard scenario additionally needs the `docker-compose.test.yml`
 * backend reachable (same one the instrumented E2E suite uses); pass its origin
 * with `-Pandroid.testInstrumentationRunnerArguments.serverUrl=http://…:7300`.
 * Results (benchmarkData.json + Perfetto traces) land under
 * `macrobenchmark/build/outputs/` and are uploaded as a CI artifact; CI never
 * gates on the absolute numbers (emulator variance) — it is a trend signal.
 */
plugins {
    // No version: the Android Gradle Plugin (which carries `com.android.test`)
    // and the Kotlin plugin are already on the build classpath via the
    // `build-logic` included build, exactly as the `mycorrhizal.android.*`
    // convention plugins apply `com.android.application` / kotlin unversioned.
    id("com.android.test")
    id("org.jetbrains.kotlin.android")
}

android {
    namespace = "com.mycorrhizal.crm.macrobenchmark"
    // Matches the app / convention-plugin AndroidConfig.COMPILE_SDK.
    compileSdk = 37

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    defaultConfig {
        // Macrobenchmark requires API 23+; the CI emulator (android-tests.yml)
        // is API 35. Aligned with the app's minSdk 26.
        minSdk = 26
        targetSdk = 37
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }

    // `com.android.test` modules default to a single `debug` build type. The
    // benchmark must run against the app's `benchmark` variant (release-shaped,
    // profileable — see app/build.gradle.kts), so mirror it here; the app-side
    // `matchingFallbacks` covers any library dependency that only has `release`.
    buildTypes {
        create("benchmark") {
            initWith(getByName("debug"))
            matchingFallbacks += listOf("release")
        }
    }

    targetProjectPath = ":app"
    // The test APK self-instruments (no separate instrumentation target app).
    experimentalProperties["android.experimental.self-instrumenting"] = true
}

// Only the `benchmark` variant is meaningful; disable the rest so
// `connectedCheck` doesn't try to run an empty `debug` connected suite.
androidComponents {
    beforeVariants(selector().all()) {
        it.enable = it.buildType == "benchmark"
    }
}

kotlin {
    compilerOptions {
        jvmTarget.set(JvmTarget.JVM_17)
    }
}

dependencies {
    implementation(libs.junit)
    implementation(libs.androidx.test.ext.junit)
    implementation(libs.androidx.test.runner)
    implementation(libs.androidx.test.uiautomator)
    implementation(libs.androidx.benchmark.macro.junit4)
}
