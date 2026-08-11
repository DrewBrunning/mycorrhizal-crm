package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.data.local.CustomLinkActionDao
import com.mycorrhizal.crm.domain.repository.CustomLinkAction as DomainCustomLinkAction
import com.mycorrhizal.crm.domain.repository.CustomLinkActionRepository
import javax.inject.Inject

class CustomLinkActionRepositoryImpl @Inject constructor(
    private val dao: CustomLinkActionDao,
) : CustomLinkActionRepository {
    override suspend fun getForProtocol(protocol: String): List<DomainCustomLinkAction> =
        dao.getForProtocol(protocol).map { it.toDomain() }

    override suspend fun getAll(): List<DomainCustomLinkAction> =
        dao.getAll().map { it.toDomain() }

    override suspend fun upsert(action: DomainCustomLinkAction) {
        dao.upsert(
            com.mycorrhizal.crm.data.local.CustomLinkAction(
                id = action.id,
                protocol = action.protocol,
                label = action.label,
                kind = action.kind,
                mimeType = action.mimeType,
                intentUriTemplate = action.intentUriTemplate,
            ),
        )
    }

    override suspend fun delete(action: DomainCustomLinkAction) {
        dao.delete(
            com.mycorrhizal.crm.data.local.CustomLinkAction(
                id = action.id,
                protocol = action.protocol,
                label = action.label,
                kind = action.kind,
                mimeType = action.mimeType,
                intentUriTemplate = action.intentUriTemplate,
            ),
        )
    }
}

private fun com.mycorrhizal.crm.data.local.CustomLinkAction.toDomain(): DomainCustomLinkAction =
    DomainCustomLinkAction(
        id = id,
        protocol = protocol,
        label = label,
        kind = kind,
        mimeType = mimeType,
        intentUriTemplate = intentUriTemplate,
    )
