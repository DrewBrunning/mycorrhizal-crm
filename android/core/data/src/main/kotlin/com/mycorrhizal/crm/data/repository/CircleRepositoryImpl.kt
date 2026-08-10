package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.data.local.CachedCircle
import com.mycorrhizal.crm.data.local.CachedCircleDao
import com.mycorrhizal.crm.data.local.CachedCircleMember
import com.mycorrhizal.crm.data.local.CachedCircleMemberDao
import com.mycorrhizal.crm.domain.repository.CircleDetail
import com.mycorrhizal.crm.domain.repository.CircleRepository
import com.mycorrhizal.crm.model.network.Circle
import com.mycorrhizal.crm.model.network.CircleInput
import com.mycorrhizal.crm.model.network.CircleMember
import com.mycorrhizal.crm.model.network.CircleMemberInput
import com.mycorrhizal.crm.network.ApiClient
import javax.inject.Inject

/**
 * Online-first circle access. Writes go to the server; successful responses
 * are mirrored into the Room cache. Membership join rows hard-delete (a join
 * row *is* its endpoints), so removal deletes the cached row outright.
 */
class CircleRepositoryImpl @Inject constructor(
    private val apiClient: ApiClient,
    private val circleDao: CachedCircleDao,
    private val memberDao: CachedCircleMemberDao,
) : CircleRepository {

    override suspend fun list(cursor: String?, limit: Int): Result<List<Circle>> {
        val result = apiClient.listCircles(cursor = cursor, limit = limit)
        val page = result.getOrNull()
        if (page != null) {
            circleDao.upsertAll(page.circles.map { it.toCached() })
        }
        return result.map { page -> page.circles }
    }

    override suspend fun getWithMembers(id: String): Result<CircleDetail> {
        val result = apiClient.getCircle(id)
        val detail = result.getOrNull()
        val circle = detail?.circle
        if (circle != null) {
            circleDao.upsert(circle.toCached())
            val members = detail.members.orEmpty()
            memberDao.deleteByCircleId(id)
            memberDao.upsertAll(members.map { it.toCached() })
            return Result.success(CircleDetail(circle = circle, members = members))
        }
        return result.map { d ->
            CircleDetail(
                circle = d.circle ?: Circle(),
                members = d.members.orEmpty(),
            )
        }
    }

    override suspend fun create(name: String): Result<Circle> {
        val result = apiClient.createCircle(CircleInput(name = name))
        result.getOrNull()?.let { circleDao.upsert(it.toCached()) }
        return result
    }

    override suspend fun rename(id: String, name: String): Result<Circle> {
        val result = apiClient.updateCircle(id, CircleInput(name = name))
        result.getOrNull()?.let { circleDao.upsert(it.toCached()) }
        return result
    }

    override suspend fun delete(id: String): Result<Unit> {
        val result = apiClient.deleteCircle(id)
        if (result.isSuccess) {
            circleDao.deleteById(id)
            memberDao.deleteByCircleId(id)
        }
        return result
    }

    override suspend fun addMember(circleId: String, vcardUid: String): Result<CircleMember> {
        val result = apiClient.addCircleMember(circleId, CircleMemberInput(memberVCardUid = vcardUid))
        result.getOrNull()?.let { memberDao.upsertAll(listOf(it.toCached())) }
        return result
    }

    override suspend fun removeMember(circleId: String, vcardUid: String): Result<Unit> {
        val result = apiClient.removeCircleMember(circleId, vcardUid)
        if (result.isSuccess) {
            memberDao.deleteMember(circleId, vcardUid)
        }
        return result
    }

    private fun Circle.toCached(): CachedCircle = CachedCircle(
        id = id,
        name = name,
        updatedAt = updatedAt,
    )

    private fun CircleMember.toCached(): CachedCircleMember = CachedCircleMember(
        id = id,
        circleId = circleId,
        memberVCardUid = memberVCardUid,
    )
}
