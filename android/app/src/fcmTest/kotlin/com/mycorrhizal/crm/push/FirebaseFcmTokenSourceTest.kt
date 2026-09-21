package com.mycorrhizal.crm.push

import android.app.Application
import com.google.android.gms.tasks.Tasks
import com.google.firebase.messaging.FirebaseMessaging
import io.mockk.every
import io.mockk.mockk
import io.mockk.mockkStatic
import io.mockk.unmockkAll
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

/**
 * Issue #1133: `FirebaseFcmTokenSource` is the obtainium/play-only
 * implementation of the [com.mycorrhizal.crm.feature.tracking.FcmTokenSource]
 * seam, so it exists only in `src/fcmTest`. The Firebase static singleton is
 * mocked — there is no real Firebase project in CI — and the returned
 * `Task<String>` is already complete, so `await()` resolves without the
 * coroutine-play-services main-thread machinery.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], application = Application::class)
class FirebaseFcmTokenSourceTest {

    @After
    fun tearDown() = unmockkAll()

    @Test
    fun `returns the FirebaseMessaging registration token`() = runBlocking {
        val messaging = mockk<FirebaseMessaging>()
        mockkStatic(FirebaseMessaging::class)
        every { FirebaseMessaging.getInstance() } returns messaging
        every { messaging.token } returns Tasks.forResult("fcm-registration-token")

        assertEquals("fcm-registration-token", FirebaseFcmTokenSource().token())
    }
}
