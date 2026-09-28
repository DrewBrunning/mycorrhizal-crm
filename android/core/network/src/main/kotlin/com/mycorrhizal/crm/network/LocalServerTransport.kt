package com.mycorrhizal.crm.network

import android.net.LocalSocket
import android.net.LocalSocketAddress
import okhttp3.Dns
import java.io.IOException
import java.io.InputStream
import java.io.OutputStream
import java.net.InetAddress
import java.net.InetSocketAddress
import java.net.Socket
import java.net.SocketAddress
import javax.net.SocketFactory

/**
 * ADR 0028 Decision 2: the `Local` server profile's transport. The embedded
 * server listens on a Unix domain socket in app-private storage — never TCP
 * loopback, which any app on the device can reach. Requests for the `Local`
 * profile are addressed to the sentinel host [LOCAL_SERVER_SENTINEL_HOST]; a
 * [ProfileAwareSocketFactory] intercepts exactly that host and speaks HTTP over
 * the socket, while [ProfileAwareDns] never lets the sentinel name reach the
 * system resolver. Every other host keeps the ordinary network path, so the app
 * keeps one OkHttpClient for both profile kinds (the ADR's "profile-aware
 * OkHttp `SocketFactory`/`Dns`").
 *
 * The `.invalid` TLD never resolves, so even a bug that let the sentinel into
 * the resolver could not reach a real network host; the release network security
 * config's one cleartext carve-out for this host is therefore harmless (ADR
 * 0028, Consequences). Without that carve-out the release config's blanket
 * cleartext ban would reject every local request before it reached this
 * factory.
 */

/** The sentinel host every `Local`-profile request is addressed to. */
const val LOCAL_SERVER_SENTINEL_HOST = "embedded.invalid"

/** The sentinel origin [com.mycorrhizal.crm.data.session.DefaultSessionManager] returns for `Local`. */
const val LOCAL_SERVER_SENTINEL_URL = "http://$LOCAL_SERVER_SENTINEL_HOST"

/**
 * Supplies the running embedded server's Unix socket path, or null when a
 * `Remote` profile is active (or the embedded server is not running). The
 * factory and DNS consult it per request; null means "use the network".
 */
fun interface LocalSocketPathProvider {
    fun socketPath(): String?
}

/** Which transport a connect attempt on a given host should use. */
internal enum class LocalTransport { LOCAL_SOCKET, NETWORK }

/**
 * The routing rule, pure so it is unit-testable without a socket: only the
 * sentinel host, and only when a live socket path is known, takes the local
 * transport. A null/blank path (Remote profile active, or the embedded server
 * not yet started) falls back to the network, which then fails the request
 * loudly rather than silently reaching a different host.
 */
internal fun selectTransport(host: String?, localSocketPath: String?): LocalTransport =
    if (host == LOCAL_SERVER_SENTINEL_HOST && !localSocketPath.isNullOrBlank()) {
        LocalTransport.LOCAL_SOCKET
    } else {
        LocalTransport.NETWORK
    }

/**
 * Resolves the sentinel host to a loopback-shaped address locally (the socket
 * factory ignores it and dials the Unix socket), and delegates every other name
 * to the system resolver. This is what keeps `embedded.invalid` from ever being
 * sent to DNS.
 */
class ProfileAwareDns(private val provider: LocalSocketPathProvider) : Dns {
    override fun lookup(hostname: String): List<InetAddress> {
        if (selectTransport(hostname, provider.socketPath()) == LocalTransport.LOCAL_SOCKET) {
            // A marker address: the host string is preserved so the socket
            // factory can recognise the sentinel in connect(endpoint).
            return listOf(InetAddress.getByAddress(hostname, LOOPBACK))
        }
        return Dns.SYSTEM.lookup(hostname)
    }

    private companion object {
        private val LOOPBACK = byteArrayOf(127, 0, 0, 1)
    }
}

/**
 * A [SocketFactory] whose sockets dial the embedded server's Unix socket only
 * for [LOCAL_SERVER_SENTINEL_HOST], and otherwise delegate to a real TCP
 * socket. Installed on the single app-scoped OkHttpClient.
 */
class ProfileAwareSocketFactory(
    private val provider: LocalSocketPathProvider,
) : SocketFactory() {

    override fun createSocket(): Socket = ProfileAwareSocket(provider)

    override fun createSocket(host: String, port: Int): Socket =
        createSocket().also { it.connect(InetSocketAddress(host, port)) }

    override fun createSocket(host: String, port: Int, localHost: InetAddress, localPort: Int): Socket =
        createSocket(host, port)

    override fun createSocket(host: InetAddress, port: Int): Socket =
        createSocket(host.hostAddress.orEmpty(), port)

    override fun createSocket(
        address: InetAddress,
        port: Int,
        localAddress: InetAddress,
        localPort: Int,
    ): Socket = createSocket(address, port)
}

/**
 * A [Socket] that is either an `android.net.LocalSocket` (sentinel host) or a
 * plain TCP socket. OkHttp dials an unconnected factory socket and then calls
 * [connect], so the branch is decided there, per connect, from the host of the
 * endpoint and the provider's current path.
 */
internal class ProfileAwareSocket(
    private val provider: LocalSocketPathProvider,
) : Socket() {

    private var network: Socket? = null
    private var local: LocalSocket? = null
    private var connected = false
    private var closed = false
    private var pendingTimeout = 0
    private var inputShutdown = false
    private var outputShutdown = false

    override fun connect(endpoint: SocketAddress?) = connect(endpoint, 0)

    override fun connect(endpoint: SocketAddress?, timeout: Int) {
        val host = (endpoint as? InetSocketAddress)?.hostString
        if (selectTransport(host, provider.socketPath()) == LocalTransport.LOCAL_SOCKET) {
            val path = provider.socketPath()
            if (path.isNullOrBlank()) {
                throw IOException("embedded.invalid addressed with no local socket path")
            }
            val ls = LocalSocket()
            ls.connect(LocalSocketAddress(path, LocalSocketAddress.Namespace.FILESYSTEM))
            if (pendingTimeout > 0) ls.soTimeout = pendingTimeout
            local = ls
        } else {
            val tcp = Socket()
            if (endpoint != null) {
                tcp.connect(endpoint, if (timeout < 0) 0 else timeout)
            }
            if (pendingTimeout > 0) tcp.soTimeout = pendingTimeout
            network = tcp
        }
        connected = true
    }

    private fun delegate(): Socket = checkNotNull(network) { "no TCP socket" }

    override fun getInputStream(): InputStream = local?.inputStream ?: delegate().getInputStream()

    override fun getOutputStream(): OutputStream = local?.outputStream ?: delegate().getOutputStream()

    override fun setSoTimeout(timeout: Int) {
        pendingTimeout = timeout
        local?.soTimeout = timeout
        network?.soTimeout = timeout
    }

    override fun getSoTimeout(): Int = local?.soTimeout ?: network?.soTimeout ?: pendingTimeout

    override fun isConnected(): Boolean = connected

    override fun isClosed(): Boolean = closed

    // OkHttp's connection-health check calls these; LocalSocket's own
    // implementations throw UnsupportedOperationException, so track state here.
    override fun isInputShutdown(): Boolean = inputShutdown

    override fun isOutputShutdown(): Boolean = outputShutdown

    override fun shutdownInput() {
        inputShutdown = true
        local?.shutdownInput()
        network?.shutdownInput()
    }

    override fun shutdownOutput() {
        outputShutdown = true
        local?.shutdownOutput()
        network?.shutdownOutput()
    }

    override fun getInetAddress(): InetAddress =
        local?.let { InetAddress.getByAddress(LOCAL_SERVER_SENTINEL_HOST, LOOPBACK) }
            ?: delegate().inetAddress

    override fun getRemoteSocketAddress(): SocketAddress =
        local?.let { InetSocketAddress(getInetAddress(), 80) } ?: delegate().remoteSocketAddress

    override fun getLocalSocketAddress(): SocketAddress? =
        local?.let { InetSocketAddress(getInetAddress(), 80) } ?: network?.localSocketAddress

    override fun getPort(): Int = network?.port ?: 80

    override fun setTcpNoDelay(on: Boolean) {
        network?.tcpNoDelay = on
    }

    override fun setKeepAlive(on: Boolean) {
        network?.keepAlive = on
    }

    override fun setSoLinger(on: Boolean, linger: Int) {
        network?.setSoLinger(on, linger)
    }

    override fun close() {
        if (closed) return
        closed = true
        local?.close()
        network?.close()
    }

    private companion object {
        private val LOOPBACK = byteArrayOf(127, 0, 0, 1)
    }
}
