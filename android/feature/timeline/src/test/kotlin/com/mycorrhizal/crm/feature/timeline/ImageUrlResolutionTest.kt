package com.mycorrhizal.crm.feature.timeline

import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.unit.dp
import coil3.ImageLoader
import coil3.SingletonImageLoader
import coil3.annotation.DelicateCoilApi
import coil3.intercept.Interceptor
import coil3.request.ErrorResult
import coil3.request.ImageResult
import com.mycorrhizal.crm.ui.LocalServerUrl
import java.util.concurrent.CopyOnWriteArrayList
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

// The v1.6.0 review (#2304) found every note, activity and gallery image blank
// on a real device: the server returns relative `/api/v1/...` URLs, and the
// image composables handed them to Coil as-is, so no request ever reached the
// server. They must resolve against the configured origin, as ContactAvatar does.
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@OptIn(DelicateCoilApi::class)
class ImageUrlResolutionTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private val requested = CopyOnWriteArrayList<Any>()

    @Before
    fun recordRequests() {
        SingletonImageLoader.setUnsafe { context ->
            ImageLoader.Builder(context)
                .components {
                    add(
                        Interceptor { chain ->
                            requested += chain.request.data
                            val offline: ImageResult = ErrorResult(
                                image = null,
                                request = chain.request,
                                throwable = IllegalStateException("no network in tests")
                            )
                            offline
                        }
                    )
                }
                .build()
        }
    }

    @After
    fun resetLoader() {
        SingletonImageLoader.reset()
    }

    private fun awaitRequest(): Any {
        composeTestRule.waitUntil(timeoutMillis = 5_000) { requested.isNotEmpty() }
        return requested.first()
    }

    @Test
    fun `a thumbnail's relative URL is requested from the configured server`() {
        composeTestRule.setContent {
            CompositionLocalProvider(LocalServerUrl provides "https://crm.example.com/") {
                ImageThumbnail(
                    model = "/api/v1/notes/4/images/9/thumbnail",
                    contentDescription = "thumb",
                    size = 64.dp
                )
            }
        }
        assertEquals("https://crm.example.com/api/v1/notes/4/images/9/thumbnail", awaitRequest())
    }

    @Test
    fun `the lightbox's relative URL is requested from the configured server`() {
        composeTestRule.setContent {
            CompositionLocalProvider(LocalServerUrl provides "https://crm.example.com") {
                ImageLightbox(
                    url = "/api/v1/activities/2/images/5/file",
                    contentDescription = "full",
                    onDismiss = {}
                )
            }
        }
        assertEquals("https://crm.example.com/api/v1/activities/2/images/5/file", awaitRequest())
    }
}
