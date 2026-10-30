package com.mycorrhizal.crm.ui.components

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.CalendarToday
import androidx.compose.material.icons.outlined.Close
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Checkbox
import androidx.compose.material3.DatePicker
import androidx.compose.material3.DatePickerDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.SelectableDates
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TimePicker
import androidx.compose.material3.rememberDatePickerState
import androidx.compose.material3.rememberTimePickerState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import com.mycorrhizal.crm.model.util.DateFieldInput
import com.mycorrhizal.crm.model.util.DateFieldMode
import com.mycorrhizal.crm.model.util.DateFieldState
import com.mycorrhizal.crm.model.util.DateFieldValue
import com.mycorrhizal.crm.model.util.TypedDate
import com.mycorrhizal.crm.ui.LocalDateFormat
import com.mycorrhizal.crm.ui.LocalIs24Hour
import com.mycorrhizal.crm.ui.R
import java.time.Instant
import java.time.LocalDate
import java.time.LocalTime
import java.time.ZoneId
import java.time.ZoneOffset

/** Why the text typed into a [DateField] was not accepted. */
private enum class DateTypingError { INVALID, OUT_OF_RANGE }

/**
 * Issue #1726 / #2207: the one date-entry component for every Android date field.
 *
 * An editable text field shows the stored value in the user's date-format (and
 * 12/24-hour) preference. A date can be **typed** in that format's digit order
 * and separator (month-name formats are typed numerically; a pasted ISO
 * `YYYY-MM-DD` is also accepted, see [DateFieldInput]). A complete, real date
 * is reported as soon as it is typed; an invalid or out-of-range entry shows an
 * inline error on Done / focus loss, keeps its text and leaves the value
 * untouched. The Material3 [DatePickerDialog] is an add-on, opened from the
 * trailing calendar icon; a picker choice replaces any half-typed text.
 *
 * [mode] decides the wire shape and whether the flow offers a time:
 *  - [DateFieldMode.DATE_ONLY] / [DateFieldMode.DATE_UTC_MIDNIGHT]:
 *    typed full dates only;
 *  - [DateFieldMode.PARTIAL]: a full date, or day+month only in the format's
 *    order (`15.01.`, `01/15`), which means "year unknown" (`--MM-DD`); the
 *    picker keeps its "Year unknown" checkbox;
 *  - [DateFieldMode.DATE_TIME] / [DateFieldMode.DATE_TIME_OPTIONAL]: typing
 *    applies to the date part and keeps the stored time; the picker dialog
 *    also offers "Add time", which opens a [TimePicker] in the same flow.
 *    Confirming without a time keeps the date-only shape.
 *
 * [allowClear] adds the clear (×) action the list filters need.
 */
@Composable
fun DateField(
    value: String?,
    onValueChange: (String) -> Unit,
    label: String,
    mode: DateFieldMode,
    modifier: Modifier = Modifier,
    enabled: Boolean = true,
    allowClear: Boolean = false,
    onClear: (() -> Unit)? = null,
    /** Earliest date a *new* selection may make (reminders' `min: today`). */
    selectableFrom: LocalDate? = null,
    testTag: String? = null
) {
    val dateFormat = LocalDateFormat.current
    val is24Hour = LocalIs24Hour.current
    val focusManager = LocalFocusManager.current
    val zone = ZoneId.systemDefault()
    val stored = remember(value, mode) { DateFieldValue.parse(value, mode, zone) }
    var showPicker by remember { mutableStateOf(false) }
    // The text being typed, or null while the field mirrors [value].
    var draft by remember { mutableStateOf<String?>(null) }
    var focused by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<DateTypingError?>(null) }
    val hasValue = !value.isNullOrBlank()
    val clear = {
        if (onClear != null) onClear() else onValueChange("")
    }
    val allowYearless = mode == DateFieldMode.PARTIAL

    fun parseTyped(text: String): TypedDate = DateFieldInput.parse(
        text = text,
        dateFormat = dateFormat,
        allowYearless = allowYearless,
        notBefore = selectableFrom,
        alwaysAllowed = stored.date.takeIf { !stored.yearUnknown }
    )

    fun report(day: TypedDate.Day) {
        val picked = DateFieldValue.pick(
            date = day.date,
            time = if (mode.allowsTime) stored.time else null,
            mode = mode,
            yearUnknown = day.yearUnknown,
            zone = zone
        )
        if (picked != value) onValueChange(picked)
    }

    // Report a complete valid date (or an emptied field) as it is typed; the
    // text itself is only normalised on Done / focus loss so the caret never moves.
    fun handleTyping(text: String) {
        draft = text
        error = null
        when (val typed = parseTyped(text)) {
            TypedDate.Empty -> if (hasValue) clear()
            is TypedDate.Day -> report(typed)
            TypedDate.Invalid, TypedDate.OutOfRange -> Unit
        }
    }

    // Settle what was typed: a valid date is shown in the preferred format, an
    // invalid one keeps its text, shows an error and never touches the value.
    fun settle() {
        val text = draft ?: return
        when (val typed = parseTyped(text)) {
            TypedDate.Empty -> draft = null

            is TypedDate.Day -> {
                report(typed)
                draft = null
            }

            TypedDate.Invalid -> error = DateTypingError.INVALID

            TypedDate.OutOfRange -> error = DateTypingError.OUT_OF_RANGE
        }
    }

    val shown = draft ?: if (focused && enabled) {
        editableText(stored, value, dateFormat, is24Hour, mode)
    } else {
        DateFieldValue.display(value, mode, dateFormat, is24Hour)
    }
    val openPicker = { showPicker = true }

    OutlinedTextField(
        value = shown,
        onValueChange = ::handleTyping,
        readOnly = !enabled,
        enabled = enabled,
        label = { Text(label) },
        placeholder = { Text(DateFieldInput.pattern(dateFormat, allowYearless)) },
        isError = error != null,
        supportingText = error?.let { problem ->
            {
                Text(
                    when (problem) {
                        DateTypingError.OUT_OF_RANGE -> stringResource(R.string.date_field_error_range)
                        DateTypingError.INVALID -> stringResource(
                            R.string.date_field_error_invalid,
                            DateFieldInput.pattern(dateFormat, allowYearless)
                        )
                    }
                )
            }
        },
        trailingIcon = if (enabled) {
            {
                DateFieldActions(
                    showClear = allowClear && hasValue,
                    onClear = {
                        draft = null
                        error = null
                        clear()
                    },
                    onOpenPicker = openPicker,
                    pickerTag = testTag?.let { "$it-picker-open" }
                )
            }
        } else {
            null
        },
        singleLine = true,
        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Text, imeAction = ImeAction.Done),
        keyboardActions = KeyboardActions(
            onDone = {
                settle()
                focusManager.clearFocus()
            }
        ),
        modifier = modifier
            .fillMaxWidth()
            .onFocusChanged {
                if (focused && !it.isFocused) settle()
                focused = it.isFocused
            }
            .then(if (testTag != null) Modifier.testTag(testTag) else Modifier)
    )

    if (showPicker) {
        DateFieldPickerDialog(
            value = value,
            mode = mode,
            selectableFrom = selectableFrom,
            onConfirm = {
                // A picker choice replaces whatever was half-typed.
                draft = null
                error = null
                onValueChange(it)
            },
            onDismiss = { showPicker = false }
        )
    }
}

/**
 * The text a focused, un-edited field offers for editing: the numeric form in
 * the user's format (month-name formats included). A date-time field edits only
 * the date part; its stored time is kept. Reduced-precision partials
 * (year-only, year+month) are shown as stored.
 */
private fun editableText(
    stored: DateFieldState,
    value: String?,
    dateFormat: String,
    is24Hour: Boolean,
    mode: DateFieldMode
): String {
    val date = stored.date
    return when {
        date == null -> ""

        stored.yearOnly != null || stored.yearMonth != null ->
            DateFieldValue.display(value, mode, dateFormat, is24Hour)

        else -> DateFieldInput.editText(date, stored.yearUnknown, dateFormat)
    }
}

@Composable
private fun DateFieldActions(
    showClear: Boolean,
    onClear: () -> Unit,
    onOpenPicker: () -> Unit,
    pickerTag: String?
) {
    Row(verticalAlignment = Alignment.CenterVertically) {
        if (showClear) {
            IconButton(onClick = onClear) {
                Icon(
                    imageVector = Icons.Outlined.Close,
                    contentDescription = stringResource(R.string.date_field_clear_date)
                )
            }
        }
        IconButton(
            onClick = onOpenPicker,
            modifier = if (pickerTag != null) Modifier.testTag(pickerTag) else Modifier
        ) {
            Icon(
                imageVector = Icons.Outlined.CalendarToday,
                contentDescription = stringResource(R.string.date_field_pick_date)
            )
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun DateFieldPickerDialog(
    value: String?,
    mode: DateFieldMode,
    selectableFrom: LocalDate?,
    onConfirm: (String) -> Unit,
    onDismiss: () -> Unit
) {
    val zone = ZoneId.systemDefault()
    val initial = remember(value, mode) { DateFieldValue.parse(value, mode, zone) }
    val initialDate = initial.date ?: LocalDate.now()
    val initialMillis = initialDate.atStartOfDay(ZoneOffset.UTC).toInstant().toEpochMilli()
    val dateState = rememberDatePickerState(
        initialSelectedDateMillis = initialMillis,
        selectableDates = if (selectableFrom == null) {
            object : SelectableDates {}
        } else {
            // The loaded date stays selectable (an overdue reminder being
            // edited), but a newly picked day must not be before the floor.
            object : SelectableDates {
                override fun isSelectableDate(utcTimeMillis: Long): Boolean {
                    if (utcTimeMillis == initialMillis) return true
                    return Instant.ofEpochMilli(
                        utcTimeMillis
                    ).atZone(ZoneOffset.UTC).toLocalDate() >= selectableFrom
                }
            }
        }
    )
    val is24Hour = LocalIs24Hour.current
    var time by remember { mutableStateOf(initial.time) }
    var yearUnknown by remember { mutableStateOf(initial.yearUnknown) }
    var showTimePicker by remember { mutableStateOf(false) }

    DatePickerDialog(
        onDismissRequest = onDismiss,
        confirmButton = {
            TextButton(
                modifier = Modifier.testTag("date-picker-confirm"),
                onClick = {
                    val millis = dateState.selectedDateMillis ?: initialMillis
                    // An untouched year-only legacy value stays year-only rather
                    // than silently becoming 1 January (issue #1726's partial rule).
                    if (
                        (initial.yearOnly != null || initial.yearMonth != null) &&
                        millis == initialMillis &&
                        time == null &&
                        !yearUnknown
                    ) {
                        onConfirm(value.orEmpty())
                    } else {
                        val date = Instant.ofEpochMilli(millis).atZone(ZoneOffset.UTC).toLocalDate()
                        onConfirm(
                            DateFieldValue.pick(
                                date = date,
                                time = if (mode.allowsTime) time else null,
                                mode = mode,
                                yearUnknown = yearUnknown,
                                zone = zone
                            )
                        )
                    }
                    onDismiss()
                }
            ) {
                Text(stringResource(R.string.action_confirm))
            }
        },
        dismissButton = {
            Row {
                if (mode.allowsTime) {
                    TextButton(
                        modifier = Modifier.testTag("date-picker-time"),
                        onClick = { showTimePicker = true }
                    ) {
                        Text(
                            time?.let { DateFieldValue.timeLabel(it, is24Hour) }
                                ?: stringResource(R.string.date_field_add_time)
                        )
                    }
                }
                TextButton(onClick = onDismiss) {
                    Text(stringResource(R.string.action_cancel))
                }
            }
        }
    ) {
        Column {
            // Typed entry lives in the field itself (in the user's date format);
            // Material3's own input mode would use the system-locale pattern.
            DatePicker(state = dateState, showModeToggle = false)
            if (mode == DateFieldMode.PARTIAL) {
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    modifier = Modifier
                        .fillMaxWidth()
                        .toggleable(
                            value = yearUnknown,
                            onValueChange = { yearUnknown = it },
                            role = Role.Checkbox
                        )
                        .testTag("date-field-year-unknown")
                ) {
                    Checkbox(checked = yearUnknown, onCheckedChange = null)
                    Text(stringResource(R.string.date_field_year_unknown))
                }
            }
        }
    }

    if (showTimePicker) {
        DateFieldTimePickerDialog(
            initial = time,
            is24Hour = is24Hour,
            onConfirm = {
                time = it
                showTimePicker = false
            },
            onRemove = {
                time = null
                showTimePicker = false
            },
            onDismiss = { showTimePicker = false }
        )
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun DateFieldTimePickerDialog(
    initial: LocalTime?,
    is24Hour: Boolean,
    onConfirm: (LocalTime) -> Unit,
    onRemove: () -> Unit,
    onDismiss: () -> Unit
) {
    val timeState = rememberTimePickerState(
        initialHour = initial?.hour ?: 12,
        initialMinute = initial?.minute ?: 0,
        is24Hour = is24Hour
    )
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.date_field_time)) },
        text = { TimePicker(state = timeState) },
        confirmButton = {
            TextButton(
                modifier = Modifier.testTag("time-picker-confirm"),
                onClick = { onConfirm(LocalTime.of(timeState.hour, timeState.minute)) }
            ) {
                Text(stringResource(R.string.action_confirm))
            }
        },
        dismissButton = {
            Row {
                if (initial != null) {
                    TextButton(
                        modifier = Modifier.testTag("time-picker-remove"),
                        onClick = onRemove
                    ) {
                        Text(stringResource(R.string.date_field_remove_time))
                    }
                }
                TextButton(onClick = onDismiss) {
                    Text(stringResource(R.string.action_cancel))
                }
            }
        }
    )
}
