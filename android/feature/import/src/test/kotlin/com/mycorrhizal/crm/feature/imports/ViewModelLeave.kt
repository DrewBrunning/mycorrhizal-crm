package com.mycorrhizal.crm.feature.imports

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.ViewModelStore

/** Clears [this] through a [ViewModelStore], exactly as popping its nav destination does. */
@Suppress("UNCHECKED_CAST")
internal fun <T : ViewModel> T.leaveScreen() {
    val self = this
    val store = ViewModelStore()
    ViewModelProvider(
        store,
        object : ViewModelProvider.Factory {
            override fun <V : ViewModel> create(modelClass: Class<V>): V = self as V
        }
    )[self.javaClass]
    store.clear()
}
