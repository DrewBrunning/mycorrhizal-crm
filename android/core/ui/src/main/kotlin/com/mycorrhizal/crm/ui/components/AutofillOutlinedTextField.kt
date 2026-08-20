package com.mycorrhizal.crm.ui.components

import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.autofill.ContentType
import androidx.compose.ui.semantics.contentType
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.input.VisualTransformation

/**
 * An [OutlinedTextField] that advertises what it holds to the Android
 * Autofill framework (T115).
 *
 * Plain M3 `OutlinedTextField`s expose no autofill hints, so Google
 * Autofill / password managers / on-device address book have nothing to
 * match a field against and never offer a prompt. This wrapper sets the
 * field's semantics [ContentType] — Compose's stable (since UI 1.8.0)
 * semantics-based Autofill API, which replaced the older experimental
 * `AutofillNode`/`LocalAutofill`/`LocalAutofillTree` trio this component
 * used to hand-roll (focus tracking, bounding-box layout callbacks, node
 * registration/disposal). The platform now does all of that itself once a
 * `contentType` is present in the semantics tree, so there's no manual
 * wiring left here at all — [contentType] null just renders a plain field.
 */
@Composable
fun AutofillOutlinedTextField(
    value: String,
    onValueChange: (String) -> Unit,
    label: String,
    contentType: ContentType?,
    modifier: Modifier = Modifier,
    singleLine: Boolean = true,
    keyboardOptions: KeyboardOptions = KeyboardOptions.Default,
    visualTransformation: VisualTransformation = VisualTransformation.None,
) {
    OutlinedTextField(
        value = value,
        onValueChange = onValueChange,
        label = { Text(label) },
        singleLine = singleLine,
        keyboardOptions = keyboardOptions,
        visualTransformation = visualTransformation,
        modifier = modifier
            .fillMaxWidth()
            .let { base ->
                if (contentType != null) {
                    base.semantics { this.contentType = contentType }
                } else {
                    base
                }
            },
    )
}
