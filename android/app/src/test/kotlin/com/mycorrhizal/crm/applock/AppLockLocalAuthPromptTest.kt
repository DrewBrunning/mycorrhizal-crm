package com.mycorrhizal.crm.applock

import android.app.Activity
import androidx.fragment.app.FragmentActivity
import com.mycorrhizal.crm.data.auth.LocalAuthOutcome
import io.mockk.coEvery
import io.mockk.every
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Test

class AppLockLocalAuthPromptTest {

    @Test
    fun `the login seam maps every app-lock outcome`() {
        assertEquals(LocalAuthOutcome.SUCCESS, AppLockAuthOutcome.Success.toLocalAuthOutcome())
        assertEquals(LocalAuthOutcome.CANCELLED, AppLockAuthOutcome.Cancelled.toLocalAuthOutcome())
        assertEquals(
            LocalAuthOutcome.NOT_AVAILABLE,
            AppLockAuthOutcome.NotAvailable.toLocalAuthOutcome()
        )
        assertEquals(LocalAuthOutcome.ERROR, AppLockAuthOutcome.Error.toLocalAuthOutcome())
    }

    @Test
    fun `a non-FragmentActivity reports unavailable without prompting`() = runTest {
        var created = 0
        val prompt = AppLockLocalAuthPrompt(
            AppUnlockPrompterFactory {
                created++
                mockk()
            }
        )

        assertEquals(
            LocalAuthOutcome.NOT_AVAILABLE,
            prompt.requestUnlock(mockk<Activity>(relaxed = true), "t", "s", null)
        )
        assertEquals(0, created)
    }

    @Test
    fun `the factory prompt's outcome is returned verbatim`() = runTest {
        val appPrompt = mockk<AppUnlockPrompter>()
        coEvery { appPrompt.requestUnlock("t", "s", "cancel") } returns AppLockAuthOutcome.Cancelled
        val prompt = AppLockLocalAuthPrompt(
            AppUnlockPrompterFactory { activity ->
                assertEquals("activity", activity.localClassName)
                appPrompt
            }
        )
        val activity = mockk<FragmentActivity>(relaxed = true) {
            every { localClassName } returns "activity"
        }

        assertEquals(
            LocalAuthOutcome.CANCELLED,
            prompt.requestUnlock(activity, "t", "s", "cancel")
        )
    }
}
