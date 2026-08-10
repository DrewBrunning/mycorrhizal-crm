# R8/ProGuard rules for Mycorrhizal Android.

# --- Moshi (codegen via @JsonClass) ---
# Keep the generated model + adapter classes referenced by the registry.
-keep class com.mycorrhizal.crm.model.network.** { *; }
-keepclassmembers class ** {
    @com.squareup.moshi.JsonClass *;
}
-dontwarn com.squareup.moshi.**

# --- Hilt / Dagger ---
-dontwarn dagger.hilt.**
-keep class dagger.hilt.android.internal.lifecycle.** { *; }

# --- Room ---
-dontwarn androidx.room.**
-keep class * extends androidx.room.RoomDatabase

# --- WorkManager (reflection over worker classes) ---
-keep class com.mycorrhizal.crm.feature.tracking.**Worker { *; }
-keep class * extends androidx.work.CoroutineWorker { *; }
-dontwarn androidx.work.**

# --- Coil ---
-dontwarn coil.**

# --- OkHttp / Okio ---
-keepattributes Signature
-keepattributes *Annotation*
-dontwarn okhttp3.**
-dontwarn okio.**

# --- Tink (EncryptedSharedPreferences) pulls compile-time-only annotations ---
-dontwarn com.google.errorprone.annotations.**
-dontwarn com.google.crypto.tink.**
-keep class com.google.crypto.tink.** { *; }
