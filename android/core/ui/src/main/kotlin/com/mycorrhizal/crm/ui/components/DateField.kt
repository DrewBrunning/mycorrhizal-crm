package com.mycorrhizal.crm.ui.components

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.selection.toggleable
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
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.unit.dp
import com.mycorrhizal.crm.model.util.DateFieldMode
import com.mycorrhizal.crm.model.util.DateFieldValue
import com.mycorrhizal.crm.ui.LocalDateFormat
import com.mycorrhizal.crm.ui.LocalIs24Hour
import com.mycorrhizal.crm.ui.R
import java.time.Instant
import java.time.LocalDate
import java.time.LocalTime
import java.time.ZoneId
import java.time.ZoneOffset

/**
 * Issue #1726: the one date-entry component for every Android date field.
 *
 * A read-only text field renders the stored value in the user's date-format
 * (and 12/24-hour) preference; tapping it opens a Material3 [DatePickerDialog].
 * [mode] decides the wire shape and whether the flow offers a time:
 *  - [DateFieldMode.DATE_ONLY] / [DateFieldMode.DATE_UTC_MIDNIGHT] /
 *    [DateFieldMode.PARTIAL]: date picker only (partial adds a "Year unknown"
 *    checkbox);
 *  - [DateFieldMode.DATE_TIME] / [DateFieldMode.DATE_TIME_OPTIONAL]: the same
 *    date dialog also offers "Add time", which opens a [TimePicker] in the
 *    same flow. Confirming without a time keeps the date-only shape.
 *
 * [allowClear] adds the clear (×) action the list filters need.
 */
@OptIn(ExperimentalMaterial3Api::class)
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
    testTag: String? = null,
) {
    val dateFormat = LocalDateFormat.current
    val is24Hour = LocalIs24Hour.current
    val display = DateFieldValue.display(value, mode, dateFormat, is24Hour)
    var showPicker by remember { mutableStateOf(false) }
    val hasValue = !value.isNullOrBlank()
    val clear = {
        if (onClear != null) onClear() else onValueChange("")
    }

    val actionsWidth = if (allowClear && hasValue) 96.dp else 48.dp
    Box(modifier = modifier.fillMaxWidth()) {
        OutlinedTextField(
            value = display,
            onValueChange = {},
            readOnly = true,
            enabled = enabled,
            label = { Text(label) },
            // The actions are drawn over the field below so the whole field
            // can be the picker's tap target without swallowing the icons.
            trailingIcon = { Spacer(Modifier.width(actionsWidth)) },
            singleLine = true,
            modifier = Modifier
                .fillMaxWidth()
                .then(if (testTag != null) Modifier.testTag(testTag) else Modifier),
        )
        if (enabled) {
            // A read-only text field still consumes taps itself, so the tap
            // target is an overlay between the field and its actions.
            Box(
                modifier = Modifier
                    .matchParentSize()
                    .clickable(onClickLabel = label) { showPicker = true },
            )
            Row(
                modifier = Modifier
                    .matchParentSize()
                    .padding(end = 4.dp),
                horizontalArrangement = Arrangement.End,
                verticalAlignment = Alignment.CenterVertically,
            ) {
                if (allowClear && hasValue) {
                    IconButton(onClick = clear, enabled = enabled) {
                        Icon(
                            imageVector = Icons.Outlined.Close,
                            contentDescription = stringResource(R.string.date_field_clear_date),
                        )
                    }
                }
                IconButton(onClick = { showPicker = true }, enabled = enabled) {
                    Icon(
                        imageVector = Icons.Outlined.CalendarToday,
                        contentDescription = stringResource(R.string.date_field_pick_date),
                    )
                }
            }
        }
    }

    if (showPicker) {
        DateFieldPickerDialog(
            value = value,
            mode = mode,
            selectableFrom = selectableFrom,
            onConfirm = onValueChange,
            onDismiss = { showPicker = false },
        )
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun DateFieldPickerDialog(
    value: String?,
    mode: DateFieldMode,
    selectableFrom: LocalDate?,
    onConfirm: (String) -> Unit,
    onDismiss: () -> Unit,
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
                    return Instant.ofEpochMilli(utcTimeMillis).atZone(ZoneOffset.UTC).toLocalDate() >= selectableFrom
                }
            }
        },
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
                                zone = zone,
                            ),
                        )
                    }
                    onDismiss()
                },
            ) {
                Text(stringResource(R.string.action_confirm))
            }
        },
        dismissButton = {
            Row {
                if (mode.allowsTime) {
                    TextButton(
                        modifier = Modifier.testTag("date-picker-time"),
                        onClick = { showTimePicker = true },
                    ) {
                        Text(
                            time?.let { DateFieldValue.timeLabel(it, is24Hour) }
                                ?: stringResource(R.string.date_field_add_time),
                        )
                    }
                }
                TextButton(onClick = onDismiss) {
                    Text(stringResource(R.string.action_cancel))
                }
            }
        },
    ) {
        Column {
            DatePicker(state = dateState, showModeToggle = false)
            if (mode == DateFieldMode.PARTIAL) {
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    modifier = Modifier
                        .fillMaxWidth()
                        .toggleable(
                            value = yearUnknown,
                            onValueChange = { yearUnknown = it },
                            role = Role.Checkbox,
                        )
                        .testTag("date-field-year-unknown"),
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
            onDismiss = { showTimePicker = false },
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
    onDismiss: () -> Unit,
) {
    val timeState = rememberTimePickerState(
        initialHour = initial?.hour ?: 12,
        initialMinute = initial?.minute ?: 0,
        is24Hour = is24Hour,
    )
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.date_field_time)) },
        text = { TimePicker(state = timeState) },
        confirmButton = {
            TextButton(
                modifier = Modifier.testTag("time-picker-confirm"),
                onClick = { onConfirm(LocalTime.of(timeState.hour, timeState.minute)) },
            ) {
                Text(stringResource(R.string.action_confirm))
            }
        },
        dismissButton = {
            Row {
                if (initial != null) {
                    TextButton(
                        modifier = Modifier.testTag("time-picker-remove"),
                        onClick = onRemove,
                    ) {
                        Text(stringResource(R.string.date_field_remove_time))
                    }
                }
                TextButton(onClick = onDismiss) {
                    Text(stringResource(R.string.action_cancel))
                }
            }
        },
    )
}
