package com.mycorrhizal.crm.ui.components

import androidx.compose.runtime.Composable
import androidx.compose.ui.res.stringResource
import com.mycorrhizal.crm.model.network.ContactFieldGroup
import com.mycorrhizal.crm.model.network.ContactFieldKey
import com.mycorrhizal.crm.ui.R

/**
 * Issue #832: the display label for a [ContactFieldKey] toggle row (Contact
 * field settings) — reuses whatever label the contact form/detail screens
 * already show for that field, so a field's name in settings always matches
 * its name everywhere else. New keys with no prior Android UI get a new
 * string (`contact_gender`, `contact_speak_to_as`, …).
 */
@Composable
fun contactFieldLabel(key: ContactFieldKey): String = when (key) {
    ContactFieldKey.EMAILS -> stringResource(R.string.contact_email)
    ContactFieldKey.PHONES -> stringResource(R.string.contact_phone)
    ContactFieldKey.ADDRESSES -> stringResource(R.string.contact_address)
    ContactFieldKey.LINKS -> stringResource(R.string.contact_links)
    ContactFieldKey.IMPP_ADDRESSES -> stringResource(R.string.contact_impps)
    ContactFieldKey.SOCIAL_PROFILES -> stringResource(R.string.contact_social_profiles)
    ContactFieldKey.OTHER_ONLINE_SERVICES -> stringResource(R.string.contact_other_online_services)
    ContactFieldKey.NICKNAME -> stringResource(R.string.contact_nickname)
    ContactFieldKey.GENDER -> stringResource(R.string.contact_gender)
    ContactFieldKey.BIRTHDAY -> stringResource(R.string.contact_birthday)
    ContactFieldKey.ANNIVERSARY -> stringResource(R.string.contact_anniversary)
    ContactFieldKey.ANNIVERSARIES -> stringResource(R.string.contact_anniversaries)
    ContactFieldKey.PREFIX -> stringResource(R.string.contact_prefix)
    ContactFieldKey.MIDDLE_NAME -> stringResource(R.string.contact_middle_name)
    ContactFieldKey.SUFFIX -> stringResource(R.string.contact_suffix)
    ContactFieldKey.ORGANIZATIONS -> stringResource(R.string.contact_organization)
    ContactFieldKey.TITLES -> stringResource(R.string.contact_job_titles)
    ContactFieldKey.HOW_WE_MET -> stringResource(R.string.contact_how_we_met)
    ContactFieldKey.WORK_INFORMATION -> stringResource(R.string.contact_work_information)
    ContactFieldKey.CONTACT_INFORMATION -> stringResource(R.string.contact_contact_information)
    ContactFieldKey.SPEAK_TO_AS -> stringResource(R.string.contact_speak_to_as)
    ContactFieldKey.PERSONAL_INFO -> stringResource(R.string.contact_personal_info)
    ContactFieldKey.KEYWORDS -> stringResource(R.string.contact_keywords)
    ContactFieldKey.CARD_NOTES -> stringResource(R.string.contact_notes)
    ContactFieldKey.PREFERRED_LANGUAGES -> stringResource(R.string.contact_preferred_languages)
    ContactFieldKey.CARD_KIND -> stringResource(R.string.contact_card_kind_label)
    ContactFieldKey.LANGUAGE -> stringResource(R.string.contact_language)
}

@Composable
fun contactFieldGroupLabel(group: ContactFieldGroup): String = when (group) {
    ContactFieldGroup.COMMUNICATION -> stringResource(R.string.settings_contact_fields_group_communication)
    ContactFieldGroup.NAME -> stringResource(R.string.settings_contact_fields_group_name)
    ContactFieldGroup.WORK -> stringResource(R.string.settings_contact_fields_group_work)
    ContactFieldGroup.PERSONAL -> stringResource(R.string.settings_contact_fields_group_personal)
    ContactFieldGroup.MYCORRHIZAL -> stringResource(R.string.settings_contact_fields_group_mycorrhizal)
}
