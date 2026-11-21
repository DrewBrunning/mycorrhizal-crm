package com.mycorrhizal.crm.e2e

import android.graphics.Bitmap
import android.graphics.Canvas
import android.util.Log
import android.view.View
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.google.android.apps.common.testing.accessibility.framework.AccessibilityCheckPreset
import com.google.android.apps.common.testing.accessibility.framework.AccessibilityCheckResult.AccessibilityCheckResultType
import com.google.android.apps.common.testing.accessibility.framework.AccessibilityHierarchyCheckResult
import com.google.android.apps.common.testing.accessibility.framework.Parameters
import com.google.android.apps.common.testing.accessibility.framework.uielement.AccessibilityHierarchyAndroid
import com.google.android.apps.common.testing.accessibility.framework.utils.contrast.BitmapImage
import java.util.Locale
import org.junit.Assert.fail
import org.junit.Test
import org.junit.runner.RunWith

/**
 * On-demand Google Accessibility Test Framework (ATF) probe: the engine behind
 * the Accessibility Scanner app, run against the real app and the real
 * `docker-compose.test.yml` backend.
 *
 * This is deliberately *not* part of the PR gate, and the 2026-11-21 evaluation
 * run is why it must not be promoted yet: on a 100% Compose app, ATF 3.1.2
 * returns `NOT_RUN` for virtually every element. Compose exposes its content as
 * `AccessibilityNodeInfo` virtual nodes, and the checks need View-backed
 * metadata Compose does not supply — text colour and size (contrast), a View to
 * measure (touch target), and view order (traversal order). The run over login,
 * dashboard, contacts, a contact detail and settings produced 235 NOT_RUN
 * results for `TextContrastCheck`, 199 for `TouchTargetSizeCheck`, 64 for
 * `TraversalOrderCheck`, and only two actionable results: a false positive on
 * the Compose host view and a real duplicate "Add" announcement.
 *
 * The probe is kept because it costs nothing outside the manual `android-a11y-scan`
 * job and re-running it is how a future ATF version gets evaluated: a run whose
 * NOT_RUN counts collapse is the signal that contrast and the node-info checks
 * have become usable here. Until then, do not schedule it and do not treat a
 * red run as a regression in the app.
 *
 * It uses the hierarchy API rather than the legacy `AccessibilityValidator`
 * (which walks `View`s and reports a single unhelpful finding on the Compose
 * host view), and reads the sealed `AccessibilityNodeInfo` tree from
 * `UiAutomation`, since a node from `View.createAccessibilityNodeInfo()` cannot
 * be traversed (unsealed). It runs the full `AccessibilityCheckPreset.LATEST`
 * with no suppressions on purpose.
 *
 * Run it with:
 *
 * ```
 * cd android && ./gradlew :app:connectedObtainiumDebugAndroidTest \
 *   -Pandroid.testInstrumentationRunnerArguments.serverUrl=http://127.0.0.1:7300 \
 *   -Pandroid.testInstrumentationRunnerArguments.requireReferences=true \
 *   -Pandroid.testInstrumentationRunnerArguments.class=com.mycorrhizal.crm.e2e.AccessibilitySweepTest
 * ```
 *
 * CI: the `android-a11y-scan` job in `android-tests.yml`, manual dispatch only.
 */
@RunWith(AndroidJUnit4::class)
@OnDemandA11yScan
class AccessibilitySweepTest : E2eBaseTest() {

    private val findings = linkedMapOf<String, MutableList<String>>()
    private val notRun = linkedMapOf<String, Int>()
    private var scannedScreens = 0

    @Test
    fun scanMainScreens() {
        // Login is the first screen every user sees, and it is the only one the
        // E2E setup normally skips past.
        clearSession()
        scan("login")
        loginViaUi()

        waitForText("Dashboard")
        scan("dashboard")

        val given = uniqueName("A11y")
        val surname = "Scan"
        val displayName = "$given $surname"
        createTestContact(given, surname)

        navigateViaDrawer("Contacts")
        waitForText("Search contacts")
        scan("contacts")

        searchFor(given)
        waitForText(displayName)
        compose.onNodeWithText(displayName).performClick()
        waitForText(displayName)
        waitForContentDescription("Edit contact")
        scan("contact-detail")

        // The detail screen's top bar is a Back arrow; the drawer only exists on
        // top-level destinations.
        clickBack()
        navigateViaDrawer("Settings")
        waitForText("Account")
        scan("settings")

        reportFindings()
    }

    /**
     * Scans the activity's window. Compose Dialog windows are separate windows
     * and are not covered here yet; the screen body is what this first run is
     * for.
     */
    private fun scan(label: String) {
        // The caller's wait* helper already proved the screen's content exists;
        // this drains any in-flight recomposition before ATF reads the tree.
        compose.waitForIdle()

        val root: View = compose.activity.window.decorView
        // Build from the sealed AccessibilityNodeInfo tree the system serves to
        // an accessibility client, not from the View tree: Compose renders into
        // one AndroidComposeView, and only the node-info path includes its
        // virtual (per-semantics-node) children. A node obtained directly from
        // View.createAccessibilityNodeInfo() is unsealed and cannot be traversed.
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        val rootNode = instrumentation.uiAutomation.rootInActiveWindow
        val hierarchy = if (rootNode != null) {
            AccessibilityHierarchyAndroid
                .newBuilder(rootNode, instrumentation.targetContext)
                .build()
        } else {
            AccessibilityHierarchyAndroid.newBuilder(root).build()
        }
        val parameters = Parameters().apply {
            // TextContrastCheck samples this capture; without it the check has
            // no pixels to evaluate.
            putScreenCapture(BitmapImage(capture(root)))
        }
        val checks = AccessibilityCheckPreset.getAccessibilityHierarchyChecksForPreset(
            AccessibilityCheckPreset.LATEST
        )

        scannedScreens++
        if (scannedScreens == 1) {
            Log.i(TAG, "checks in LATEST: ${checks.joinToString { it.javaClass.simpleName }}")
        }
        var found = 0
        for (check in checks) {
            val name = check.javaClass.simpleName
            val results = check.runCheckOnHierarchy(hierarchy, null, parameters)
            Log.i(TAG, "[$label] $name -> ${results.groupingBy { it.type }.eachCount()}")
            notRun[name] = (notRun[name] ?: 0) + results.count {
                it.type == AccessibilityCheckResultType.NOT_RUN
            }
            for (result in results) {
                if (result.type != AccessibilityCheckResultType.ERROR &&
                    result.type != AccessibilityCheckResultType.WARNING
                ) {
                    continue
                }
                found++
                record(label, name, result)
            }
        }
        Log.i(TAG, "[$label] $found ATF finding(s) across ${checks.size} checks")
    }

    private fun capture(root: View): Bitmap {
        val bitmap = Bitmap.createBitmap(root.width, root.height, Bitmap.Config.ARGB_8888)
        root.draw(Canvas(bitmap))
        return bitmap
    }

    /**
     * `toString()` carries the result's type, message and the element it points
     * at without touching ATF's Checker-Framework-annotated getters (whose
     * annotation artifact is not on this module's classpath).
     */
    private fun record(screen: String, check: String, result: AccessibilityHierarchyCheckResult) {
        // A result with no message cannot be triaged. ATF 3.1.2 produces one for
        // the Compose host view, which has no View-backed element descriptor;
        // every real finding carries text.
        val message = result.getMessage(Locale.getDefault())?.toString()
        if (message.isNullOrBlank()) {
            Log.i(TAG, "[$screen] ignored an unlabelled $check result")
            return
        }
        val line = "[$screen] ${result.type} $check: $message"
        findings.getOrPut(check) { mutableListOf() } += line
        Log.w(TAG, line)
    }

    private fun reportFindings() {
        if (findings.isEmpty()) {
            Log.i(TAG, "ATF scan clean over $scannedScreens screen(s)")
            return
        }

        val total = findings.values.sumOf { it.size }
        val report = buildString {
            appendLine("ATF scan found $total issue(s) over $scannedScreens screen(s).")
            appendLine("Full preset, no suppressions: this is the tuning input, not a gate.")
            appendLine()
            appendLine("Checks that could not evaluate the Compose tree (NOT_RUN results):")
            notRun.filterValues { it > 0 }.forEach { (check, count) ->
                appendLine("  - $check: $count")
            }
            findings.forEach { (check, lines) ->
                appendLine()
                appendLine("$check (${lines.size})")
                lines.forEach { appendLine("  - $it") }
            }
        }
        fail(report)
    }

    private companion object {
        const val TAG = "AtfScan"
    }
}
