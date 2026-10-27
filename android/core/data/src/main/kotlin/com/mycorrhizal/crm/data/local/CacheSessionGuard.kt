package com.mycorrhizal.crm.data.local

import java.util.concurrent.atomic.AtomicLong
import javax.inject.Inject
import javax.inject.Singleton
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

/**
 * Issue #2065: keeps an in-flight repository call from writing the previous
 * account's data into the Room mirror after the session ended.
 *
 * A repository captures [epoch] before its network call and writes through
 * [writeIfCurrent]; [LocalDataCleaner] wipes through [wipe]. Both take the same
 * mutex, so a write either lands before the wipe (and is wiped) or sees the
 * advanced epoch and is dropped. Process-lifetime session state such as the
 * contact feed cursor compares [epoch] to detect that the account changed.
 */
@Singleton
class CacheSessionGuard @Inject constructor() {
    private val current = AtomicLong(0L)
    private val mutex = Mutex()

    fun epoch(): Long = current.get()

    /** Marks a session boundary; everything captured earlier is now stale. */
    fun advance() {
        current.incrementAndGet()
    }

    /** Runs [block] only if no session boundary passed since [captured]; else returns null. */
    suspend fun <T> writeIfCurrent(captured: Long, block: suspend () -> T): T? =
        mutex.withLock { if (current.get() == captured) block() else null }

    /** Advances the epoch and runs [block] while no cache write can interleave. */
    suspend fun wipe(block: suspend () -> Unit) {
        mutex.withLock {
            advance()
            block()
        }
    }
}
