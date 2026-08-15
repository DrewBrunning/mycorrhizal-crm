package com.mycorrhizal.crm.ui.components

import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.ui.ExperimentalComposeUiApi
import androidx.compose.ui.Modifier
import androidx.compose.ui.autofill.AutofillNode
import androidx.compose.ui.autofill.AutofillType
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.layout.boundsInRoot
import androidx.compose.ui.layout.onGloballyPositioned
import androidx.compose.ui.platform.LocalAutofill
import androidx.compose.ui.platform.LocalAutofillTree

/**
 * An [OutlinedTextField] that advertises what it holds to the Android
 * Autofill framework (T115).
 *
 * Plain M3 `OutlinedTextField`s expose no autofill hints, so Google
 * Autofill / password managers / on-device address book have nothing to
 * match a field against and never offer a prompt. This wrapper wires
 * Compose's public (still experimental) `AutofillNode`/`LocalAutofill`/
 * `LocalAutofillTree` API: it registers a node carrying [autofillType]
 * with the composition's autofill tree, tracks the field's bounding box as
 * layout runs, requests autofill when the field gains focus and cancels it
 * on focus loss. A framework fill arrives as [onFill] — the same
 * `onValueChange` the field uses — so it round-trips through the normal
 * state/validation path exactly like a keystroke.
 *
 * Guarded rather than fatal: on API < 26, or where no Autofill service is
 * available, `LocalAutofill.current` is null and everything above is a
 * no-op (the field still renders as a plain [OutlinedTextField]). The
 * `requestAutofillForNode` call is skipped until the field has actually
 * been positioned, because the framework errors on a null bounding box.
 */
@OptIn(ExperimentalComposeUiApi::class)
@Composable
fun AutofillOutlinedTextField(
    value: String,
    onValueChange: (String) -> Unit,
    label: String,
    autofillType: AutofillType?,
    modifier: Modifier = Modifier,
    singleLine: Boolean = true,
    keyboardOptions: KeyboardOptions = KeyboardOptions.Default,
) {
    val autofill = LocalAutofill.current
    val autofillTree = LocalAutofillTree.current
    // The node's onFill is fixed at construction; route through the latest
    // lambda so a recreated onValueChange (e.g. one capturing current state)
    // is always honored without rebuilding the node (and churning its id).
    val latestFill by rememberUpdatedState(onValueChange)
    val node = remember(autofillType) {
        AutofillNode(
            autofillTypes = listOfNotNull(autofillType),
            onFill = { latestFill(it) },
        )
    }
    DisposableEffect(node) {
        autofillTree += node
        onDispose {
            autofill?.cancelAutofillForNode(node)
            autofillTree.children.remove(node.id)
        }
    }
    OutlinedTextField(
        value = value,
        onValueChange = onValueChange,
        label = { Text(label) },
        singleLine = singleLine,
        keyboardOptions = keyboardOptions,
        modifier = modifier
            .fillMaxWidth()
            // boundingBox is a plain mutable field on AutofillNode, not Compose
            // state — writing it here does NOT trigger recomposition, which is
            // what the contact form wants (this fires on every scroll/relayout).
            .onGloballyPositioned { coords -> node.boundingBox = coords.boundsInRoot() }
            .onFocusChanged { focus ->
                if (focus.isFocused && node.boundingBox != null) {
                    autofill?.requestAutofillForNode(node)
                } else if (!focus.isFocused) {
                    autofill?.cancelAutofillForNode(node)
                }
            },
    )
}
