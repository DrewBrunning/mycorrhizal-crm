package com.mycorrhizal.crm.feature.imports

import android.provider.ContactsContract.CommonDataKinds.Phone
import android.provider.ContactsContract.CommonDataKinds.StructuredPostal
import com.mycorrhizal.crm.model.network.Address
import com.mycorrhizal.crm.model.network.AddressComponent
import com.mycorrhizal.crm.model.network.Anniversary
import com.mycorrhizal.crm.model.network.AnniversaryDate
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactRecordInput
import com.mycorrhizal.crm.model.network.Email
import com.mycorrhizal.crm.model.network.Name
import com.mycorrhizal.crm.model.network.NameComponent
import com.mycorrhizal.crm.model.network.Organization
import com.mycorrhizal.crm.model.network.PartialDate
import com.mycorrhizal.crm.model.network.Phone as CardPhone

/**
 * Maps a device contact to the neutral Card/ContactRecordInput (T57 §7.3).
 * The name is split into given/surname components by the last whitespace;
 * TYPE_MOBILE phones get the `cell` feature (which drives the SMS action).
 */
object DeviceContactMapper {

    fun toInput(device: DeviceContact): ContactRecordInput {
        val name = buildName(device.displayName)
        val phones = device.phones.map { (number, type) ->
            val features = if (type == Phone.TYPE_MOBILE) listOf("cell") else emptyList()
            val label = when (type) {
                Phone.TYPE_MOBILE -> "mobile"
                Phone.TYPE_HOME -> "home"
                Phone.TYPE_WORK -> "work"
                Phone.TYPE_FAX_WORK -> "fax"
                else -> null
            }
            CardPhone(
                number = number,
                features = features.ifEmpty { null },
                label = label,
            )
        }
        val emails = device.emails.map { Email(address = it) }
        val addresses = device.addresses.map { toAddress(it) }
        val card = Card(
            name = name,
            phones = phones.ifEmpty { null },
            emails = emails.ifEmpty { null },
            addresses = addresses.ifEmpty { null },
            organizations = device.organization?.let {
                listOf(Organization(name = it))
            },
            anniversaries = device.birthday?.let {
                listOf(
                    Anniversary(
                        kind = "birth",
                        date = AnniversaryDate(partial = parsePartialDate(it)),
                    ),
                )
            },
        )
        return ContactRecordInput(card = card)
    }

    private fun buildName(displayName: String?): Name? {
        val full = displayName?.trim()?.takeIf { it.isNotBlank() } ?: return null
        val split = full.indexOfLast { it == ' ' }
        return if (split > 0) {
            Name(
                full = full,
                components = listOf(
                    NameComponent(kind = "given", value = full.substring(0, split)),
                    NameComponent(kind = "surname", value = full.substring(split + 1)),
                ),
            )
        } else {
            Name(full = full, components = listOf(NameComponent(kind = "given", value = full)))
        }
    }

    /**
     * Maps a device-structured address to the registry's real component kinds — "name"
     * (street) and "locality" (city), not the intuitive-but-wrong "street"/"city" (T67 Bug B).
     * See `contactmodel/model.go:167-176`. TYPE_HOME/TYPE_WORK carry through into `contexts`
     * using the neutral private/work vocabulary (`vcard4/adapter.go`'s ctx2type: home<->private),
     * the same pair phones already use for their own home/work distinction.
     */
    private fun toAddress(device: DeviceAddress): Address {
        val components = buildList {
            device.street?.let { add(AddressComponent(kind = "name", value = it)) }
            device.city?.let { add(AddressComponent(kind = "locality", value = it)) }
            device.region?.let { add(AddressComponent(kind = "region", value = it)) }
            device.postcode?.let { add(AddressComponent(kind = "postcode", value = it)) }
            device.country?.let { add(AddressComponent(kind = "country", value = it)) }
        }
        val contexts = when (device.type) {
            StructuredPostal.TYPE_HOME -> listOf("private")
            StructuredPostal.TYPE_WORK -> listOf("work")
            else -> null
        }
        return Address(
            components = components.ifEmpty { null },
            full = device.formattedAddress,
            contexts = contexts,
        )
    }

    private fun parsePartialDate(iso: String): PartialDate? {
        val m = Regex("""(\d{4})-(\d{2})-(\d{2})""").find(iso) ?: return null
        return PartialDate(
            year = m.groupValues[1].toInt(),
            month = m.groupValues[2].toInt(),
            day = m.groupValues[3].toInt(),
        )
    }
}
