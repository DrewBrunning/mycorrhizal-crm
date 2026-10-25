package com.mycorrhizal.crm.feature.contacts

import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.data.session.SessionManager
import com.mycorrhizal.crm.domain.profile.ServerProfile
import com.mycorrhizal.crm.domain.profile.ServerProfileKind
import com.mycorrhizal.crm.domain.repository.AuthRepository
import com.mycorrhizal.crm.domain.repository.CircleRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.MapRepository
import com.mycorrhizal.crm.domain.repository.SessionState
import com.mycorrhizal.crm.domain.repository.TagRepository
import com.mycorrhizal.crm.model.network.CRMEnvelope
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.Name
import com.mycorrhizal.crm.model.network.NameComponent
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.coEvery
import io.mockk.every
import io.mockk.mockk
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

/** Issue #1751: [ContactFormState.isDirty] drives the unsaved-changes guard. */
class ContactFormDirtyStateTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val contactRepository = mockk<ContactRepository>()
    private val circleRepository = mockk<CircleRepository>()
    private val tagRepository = mockk<TagRepository>()
    private val authRepository = mockk<AuthRepository>()
    private val mapRepository = mockk<MapRepository>()
    private val activeProfile = MutableStateFlow<ServerProfile?>(
        ServerProfile("r1", ServerProfileKind.Remote("https://home.example"), "Home")
    )
    private val sessionManager = mockk<SessionManager>(relaxed = true) {
        every { observeActiveProfile() } returns activeProfile
    }

    private fun createViewModel(id: Int? = null): ContactFormViewModel {
        coEvery { circleRepository.list(any(), any()) } returns Result.success(emptyList())
        coEvery { tagRepository.list(any(), any()) } returns Result.success(emptyList())
        coEvery { circleRepository.circlesForContact(any()) } returns Result.success(emptyList())
        coEvery { tagRepository.tagsForContact(any()) } returns Result.success(emptyList())
        every { authRepository.observeSession() } returns flowOf(SessionState())
        return ContactFormViewModel(
            contactRepository,
            circleRepository,
            tagRepository,
            authRepository,
            mapRepository,
            sessionManager,
            if (id == null) SavedStateHandle() else SavedStateHandle(mapOf("contactId" to id))
        )
    }

    @Test
    fun `a new form starts clean`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = createViewModel()
        advanceUntilIdle()

        assertFalse(vm.uiState.value.isDirty)
    }

    @Test
    fun `typing marks the form dirty and clearing it marks it clean again`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        val vm = createViewModel()
        advanceUntilIdle()

        vm.onNicknameChange("Caz")
        assertTrue(vm.uiState.value.isDirty)

        vm.onNicknameChange("")
        assertFalse(vm.uiState.value.isDirty)
    }

    @Test
    fun `editing a list field, a chip and a toggle each mark the form dirty`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        val vm = createViewModel()
        advanceUntilIdle()

        vm.onKeywordsChange(listOf("climbing"))
        assertTrue(vm.uiState.value.isDirty)

        vm.onKeywordsChange(emptyList())
        assertFalse(vm.uiState.value.isDirty)

        vm.onCircleToggle("friends")
        assertTrue(vm.uiState.value.isDirty)

        vm.onCircleToggle("friends")
        assertFalse(vm.uiState.value.isDirty)

        vm.onAmbientChange(true)
        assertTrue(vm.uiState.value.isDirty)
    }

    @Test
    fun `async reference loads do not make an untouched form dirty`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        // Options, the active profile and the session's field toggles all arrive after the
        // baseline is taken; none of them is something the user typed.
        val vm = createViewModel()
        advanceUntilIdle()

        assertFalse(vm.uiState.value.isDirty)
    }

    @Test
    fun `a loaded edit is clean until the user changes something`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        coEvery { contactRepository.getContact(5) } returns Result.success(
            ContactRecordResponse(
                id = 5,
                uid = "u5",
                card = Card(
                    name = Name(
                        full = "Dana White",
                        components = listOf(NameComponent(kind = "given", value = "Dana"))
                    )
                ),
                crm = CRMEnvelope(circles = listOf("friends"))
            )
        )

        val vm = createViewModel(5)
        advanceUntilIdle()

        assertFalse(vm.uiState.value.isDirty)

        vm.onGivenNameChange("Dana W")
        assertTrue(vm.uiState.value.isDirty)

        vm.onGivenNameChange("Dana")
        assertFalse(vm.uiState.value.isDirty)
    }
}
