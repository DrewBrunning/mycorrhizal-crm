package com.mycorrhizal.crm.e2e

/**
 * Marks the tests that only the on-demand Accessibility Test Framework scan
 * runs. `android-tests.yml`'s `android-e2e` jobs pass the fully-qualified name
 * to the instrumentation runner's `notAnnotation` argument, so the required
 * emulator gate never pays for the scan; the `android-a11y-scan` job runs it
 * with `-e annotation`.
 */
@Retention(AnnotationRetention.RUNTIME)
@Target(AnnotationTarget.CLASS)
annotation class OnDemandA11yScan
