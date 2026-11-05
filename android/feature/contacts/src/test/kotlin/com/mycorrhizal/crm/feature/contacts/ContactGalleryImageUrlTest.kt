package com.mycorrhizal.crm.feature.contacts

import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.test.junit4.v2.createComposeRule
import coil3.ImageLoader
import coil3.SingletonImageLoader
import coil3.annotation.DelicateCoilApi
import coil3.intercept.Interceptor
import coil3.request.ErrorResult
import coil3.request.ImageResult
import com.mycorrhizal.crm.model.network.ContactPhoto
import com.mycorrhizal.crm.ui.LocalServerUrl
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
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

// The v1.6.0 review (#2304): gallery tiles and the crop preview handed the
// server's relative thumbnail URL to Coil as-is, so every photo rendered blank
// on a real device. They must resolve against the configured origin.
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@OptIn(DelicateCoilApi::class)
class ContactGalleryImageUrlTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private val requested = CopyOnWriteArrayList<Any>()
    private val thumb = "/api/v1/contacts/3/images/7/thumbnail"
    private val photo = ContactPhoto(id = 7, thumbnailUrl = thumb, width = 800, height = 600)

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

    @Test
    fun `a gallery tile requests its thumbnail from the configured server`() {
        composeTestRule.setContent {
            CompositionLocalProvider(LocalServerUrl provides "https://crm.example.com") {
                MycorrhizalTheme {
                    ContactGalleryScreenContent(
                        uiState = ContactGalleryUiState(photos = listOf(photo)),
                        onAddClick = {},
                        onDelete = {},
                        onSetMain = {},
                        onClearMain = {},
                        onAdopt = {},
                        onCrop = { _, _ -> },
                        onSetSensitivity = { _, _ -> }
                    )
                }
            }
        }
        composeTestRule.waitUntil(timeoutMillis = 5_000) { requested.isNotEmpty() }
        assertEquals(listOf("https://crm.example.com$thumb"), requested.distinct())
    }

    @Test
    fun `the crop preview requests its thumbnail from the configured server`() {
        composeTestRule.setContent {
            CompositionLocalProvider(LocalServerUrl provides "https://crm.example.com") {
                MycorrhizalTheme {
                    ImageCropDialog(photo = photo, onSave = {}, onClear = {}, onDismiss = {})
                }
            }
        }
        composeTestRule.waitUntil(timeoutMillis = 5_000) { requested.isNotEmpty() }
        assertEquals(listOf("https://crm.example.com$thumb"), requested.distinct())
    }
}
