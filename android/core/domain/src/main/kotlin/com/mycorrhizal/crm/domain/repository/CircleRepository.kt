package com.mycorrhizal.crm.domain.repository

import com.mycorrhizal.crm.model.network.Circle
import com.mycorrhizal.crm.model.network.CircleMember

/**
 * Circle data access. Online-first: writes go to the server and the returned
 * records are mirrored into the local cache. Circles use UUID string PKs.
 */
interface CircleRepository {
    /** All circles, cursor-paginated. */
    suspend fun list(cursor: String? = null, limit: Int = 100): Result<List<Circle>>

    /** A circle with its members. */
    suspend fun getWithMembers(id: String): Result<CircleDetail>

    /** Create a circle; returns the created circle. */
    suspend fun create(name: String): Result<Circle>

    /** Rename a circle; returns the updated circle. */
    suspend fun rename(id: String, name: String): Result<Circle>

    /** Delete a circle (hard delete). */
    suspend fun delete(id: String): Result<Unit>

    /** Add a member (by contact VCard UID) to a circle. */
    suspend fun addMember(circleId: String, vcardUid: String): Result<CircleMember>

    /** Remove a member from a circle (hard delete join row). */
    suspend fun removeMember(circleId: String, vcardUid: String): Result<Unit>
}

data class CircleDetail(
    val circle: Circle,
    val members: List<CircleMember>,
)
