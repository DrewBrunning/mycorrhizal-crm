package com.mycorrhizal.crm.feature.map

import com.mycorrhizal.crm.domain.repository.MapRepository
import com.mycorrhizal.crm.model.network.ContactMapPoint
import com.mycorrhizal.crm.model.network.ContactMapResponse
import com.mycorrhizal.crm.model.network.LatLng
import com.mycorrhizal.crm.model.network.MapConfig
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

class MapViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val repository = mockk<MapRepository>()

    private fun point(contactId: Int, addressId: String, coordinates: String, name: String = "Ada") =
        ContactMapPoint(
            contactId = contactId,
            contactName = name,
            addressId = addressId,
            label = "label-$addressId",
            coordinates = coordinates,
        )

    private fun stub(
        points: List<ContactMapPoint>? = emptyList(),
        truncated: Boolean = false,
        style: String = "https://tiles.example/s",
    ) {
        coEvery { repository.getMapConfig() } returns Result.success(MapConfig(style))
        coEvery { repository.getContactMap() } returns
            Result.success(ContactMapResponse(points = points, truncated = truncated))
    }

    @Test
    fun `loads the style and plots every valid point`() = runTest(mainDispatcherRule.testDispatcher) {
        stub(points = listOf(point(1, "a", "geo:51.5,-0.12"), point(2, "b", "geo:40,-74", name = "Grace")))

        val vm = MapViewModel(repository)
        assertTrue(vm.uiState.value.isLoading)
        advanceUntilIdle()

        val state = vm.uiState.value
        assertFalse(state.isLoading)
        assertNull(state.error)
        assertEquals("https://tiles.example/s", state.styleUrl)
        assertEquals(listOf("1/a", "2/b"), state.points.map { it.key })
        assertEquals(LatLng(51.5, -0.12), state.points[0].position)
        assertEquals("Grace", state.points[1].contactName)
        assertFalse(state.truncated)
    }

    @Test
    fun `skips a point whose coordinates the client cannot parse`() = runTest(mainDispatcherRule.testDispatcher) {
        stub(points = listOf(point(1, "bad", "geo:999,0"), point(2, "ok", "geo:1,2")))

        val vm = MapViewModel(repository)
        advanceUntilIdle()

        assertEquals(listOf("2/ok"), vm.uiState.value.points.map { it.key })
    }

    @Test
    fun `a null points list is an empty map, not a crash`() = runTest(mainDispatcherRule.testDispatcher) {
        stub(points = null)

        val vm = MapViewModel(repository)
        advanceUntilIdle()

        assertTrue(vm.uiState.value.points.isEmpty())
        assertNull(vm.uiState.value.error)
    }

    @Test
    fun `surfaces the server's truncation flag`() = runTest(mainDispatcherRule.testDispatcher) {
        stub(points = listOf(point(1, "a", "geo:1,2")), truncated = true)

        val vm = MapViewModel(repository)
        advanceUntilIdle()

        assertTrue(vm.uiState.value.truncated)
    }

    @Test
    fun `a config failure shows an error and no points`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.getMapConfig() } returns Result.failure(ApiError.Network(java.io.IOException("x")))
        coEvery { repository.getContactMap() } returns Result.success(ContactMapResponse())

        val vm = MapViewModel(repository)
        advanceUntilIdle()

        val state = vm.uiState.value
        assertFalse(state.isLoading)
        assertEquals("No connection", state.error)
        assertTrue(state.points.isEmpty())
        assertNull(state.styleUrl)
    }

    @Test
    fun `a points failure shows an error even when the config loaded`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.getMapConfig() } returns Result.success(MapConfig("s"))
        coEvery { repository.getContactMap() } returns Result.failure(ApiError.Timeout(java.net.SocketTimeoutException("x")))

        val vm = MapViewModel(repository)
        advanceUntilIdle()

        assertEquals("Request timed out", vm.uiState.value.error)
    }

    @Test
    fun `retry reloads and clears the error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.getMapConfig() } returns Result.failure(ApiError.Network(java.io.IOException("x")))
        coEvery { repository.getContactMap() } returns Result.success(ContactMapResponse())
        val vm = MapViewModel(repository)
        advanceUntilIdle()
        assertEquals("No connection", vm.uiState.value.error)

        stub(points = listOf(point(1, "a", "geo:1,2")))
        vm.load()
        advanceUntilIdle()

        assertNull(vm.uiState.value.error)
        assertEquals(1, vm.uiState.value.points.size)
        coVerify(exactly = 2) { repository.getMapConfig() }
    }

    @Test
    fun `selecting a marker exposes it and a reload clears the selection`() = runTest(mainDispatcherRule.testDispatcher) {
        stub(points = listOf(point(1, "a", "geo:1,2")))
        val vm = MapViewModel(repository)
        advanceUntilIdle()

        vm.select("1/a")
        assertEquals("Ada", vm.uiState.value.selected?.contactName)
        vm.select("nope")
        assertNull(vm.uiState.value.selected)

        vm.select("1/a")
        vm.load()
        advanceUntilIdle()
        assertNull(vm.uiState.value.selectedKey)
    }

    @Test
    fun `the list toggle flips showList`() = runTest(mainDispatcherRule.testDispatcher) {
        stub()
        val vm = MapViewModel(repository)
        advanceUntilIdle()

        assertFalse(vm.uiState.value.showList)
        vm.setShowList(true)
        assertTrue(vm.uiState.value.showList)
    }

    private fun p(lat: Double, lng: Double, id: Int = 1) =
        MapPoint(id, "n", "a$id", "l", LatLng(lat, lng))

    @Test
    fun `viewportFor frames none, one and many points`() {
        assertEquals(MapViewport.World, viewportFor(emptyList()))
        assertEquals(MapViewport.Single(LatLng(1.0, 2.0)), viewportFor(listOf(p(1.0, 2.0))))
        assertEquals(
            MapViewport.Bounds(southWest = LatLng(-10.0, -20.0), northEast = LatLng(30.0, 40.0)),
            viewportFor(listOf(p(30.0, -20.0, 1), p(-10.0, 40.0, 2), p(5.0, 0.0, 3))),
        )
    }
}
