package com.mycorrhizal.crm.feature.settings

/** Issue #1880 follow-up: a notice packaged into the APK/AAB, not read from Gradle metadata. */
data class BundledNotice(val title: String, val content: String)
