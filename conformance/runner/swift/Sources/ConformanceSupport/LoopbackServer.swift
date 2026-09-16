import Foundation
#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

#if canImport(Glibc)
import Glibc
#elseif canImport(Darwin)
import Darwin
#endif

/// A request the loopback server received.
public struct ServedRequest: Sendable {
    public let method: String
    /// The path as it arrived, still percent-encoded.
    public let path: String
    /// The query as it arrived, still percent-encoded, or nil when there was none.
    public let query: String?
    public let headers: [(String, String)]
    public let body: Data
    /// When the request arrived, in nanoseconds on a monotonic clock.
    public let arrivedAt: UInt64

    public func header(_ name: String) -> String? {
        headers.first { $0.0.caseInsensitiveCompare(name) == .orderedSame }?.1
    }
}

/// What the loopback server answers a request with.
public struct ServedResponse: Sendable {
    public var status: Int
    public var headers: [(String, String)]
    public var body: Data
    /// How long to wait before answering.
    public var delay: Duration

    public init(status: Int, headers: [(String, String)] = [], body: Data = Data(), delay: Duration = .zero) {
        self.status = status
        self.headers = headers
        self.body = body
        self.delay = delay
    }
}

/// A plain HTTP/1.1 server on 127.0.0.1 that answers each request with what `respond` says, one
/// connection per request. It stands in for HEY in the conformance runner and in the transport's
/// own tests, and runs on Linux and macOS alike.
public final class LoopbackServer: @unchecked Sendable {
    private let listener: Int32
    public let port: Int
    private let respond: @Sendable (Int, ServedRequest) -> ServedResponse
    private let lock = NSLock()
    private var received: [ServedRequest] = []
    private var stopped = false

    public var baseURL: String { "http://127.0.0.1:\(port)" }

    /// Every request the server recorded, in the order they arrived.
    public var requests: [ServedRequest] {
        lock.lock()
        defer { lock.unlock() }
        return received
    }

    /// Starts a server. `respond` is given the request's index among the recorded ones and the
    /// request, and says what to answer it with.
    public init(respond: @escaping @Sendable (Int, ServedRequest) -> ServedResponse) throws {
        self.respond = respond
        #if canImport(Glibc)
        let socketType = Int32(SOCK_STREAM.rawValue)
        #else
        let socketType = SOCK_STREAM
        #endif
        let descriptor = socket(AF_INET, socketType, 0)
        guard descriptor >= 0 else { throw LoopbackError("socket failed: \(errno)") }
        var reuse: Int32 = 1
        setsockopt(descriptor, SOL_SOCKET, SO_REUSEADDR, &reuse, socklen_t(MemoryLayout<Int32>.size))

        var address = sockaddr_in()
        address.sin_family = sa_family_t(AF_INET)
        address.sin_port = 0
        address.sin_addr.s_addr = inet_addr("127.0.0.1")
        let bound = withUnsafePointer(to: &address) {
            $0.withMemoryRebound(to: sockaddr.self, capacity: 1) {
                bind(descriptor, $0, socklen_t(MemoryLayout<sockaddr_in>.size))
            }
        }
        guard bound == 0, listen(descriptor, 64) == 0 else {
            close(descriptor)
            throw LoopbackError("bind or listen failed: \(errno)")
        }
        var length = socklen_t(MemoryLayout<sockaddr_in>.size)
        _ = withUnsafeMutablePointer(to: &address) {
            $0.withMemoryRebound(to: sockaddr.self, capacity: 1) { getsockname(descriptor, $0, &length) }
        }
        listener = descriptor
        port = Int(UInt16(bigEndian: address.sin_port))

        let thread = Thread { [weak self] in self?.acceptLoop() }
        thread.stackSize = 1 << 20
        thread.start()
    }

    /// Stops accepting connections. Requests already being answered finish.
    public func stop() {
        lock.lock()
        let already = stopped
        stopped = true
        lock.unlock()
        guard !already else { return }
        shutdown(listener, Int32(SHUT_RDWR))
        close(listener)
    }

    deinit {
        stop()
    }

    private func acceptLoop() {
        while true {
            let connection = accept(listener, nil, nil)
            if connection < 0 {
                lock.lock()
                let done = stopped
                lock.unlock()
                if done { return }
                continue
            }
            var noSigPipe: Int32 = 1
            #if canImport(Darwin)
            setsockopt(connection, SOL_SOCKET, SO_NOSIGPIPE, &noSigPipe, socklen_t(MemoryLayout<Int32>.size))
            #else
            _ = noSigPipe
            #endif
            let thread = Thread { [weak self] in
                self?.serve(connection)
                close(connection)
            }
            thread.stackSize = 1 << 20
            thread.start()
        }
    }

    private func serve(_ connection: Int32) {
        var buffer = Data()
        var chunk = [UInt8](repeating: 0, count: 65_536)
        let separator = Data("\r\n\r\n".utf8)
        var headerEnd: Range<Data.Index>?
        while headerEnd == nil {
            let count = recv(connection, &chunk, chunk.count, 0)
            if count <= 0 { return }
            buffer.append(contentsOf: chunk[0..<count])
            headerEnd = buffer.range(of: separator)
        }
        guard let end = headerEnd,
              let head = String(data: buffer[buffer.startIndex..<end.lowerBound], encoding: .utf8)
        else { return }
        var lines = head.components(separatedBy: "\r\n")
        let requestLine = lines.removeFirst().split(separator: " ")
        guard requestLine.count >= 2 else { return }
        let method = String(requestLine[0])
        let target = String(requestLine[1])
        var headers: [(String, String)] = []
        for line in lines {
            guard let colon = line.firstIndex(of: ":") else { continue }
            headers.append((String(line[..<colon]), line[line.index(after: colon)...].trimmingCharacters(in: .whitespaces)))
        }
        func header(_ name: String) -> String? {
            headers.first { $0.0.caseInsensitiveCompare(name) == .orderedSame }?.1
        }
        if header("Expect")?.lowercased() == "100-continue" {
            write(connection, Data("HTTP/1.1 100 Continue\r\n\r\n".utf8))
        }
        var body = Data(buffer[end.upperBound...])
        if let length = header("Content-Length").flatMap(Int.init) {
            while body.count < length {
                let count = recv(connection, &chunk, min(chunk.count, length - body.count), 0)
                if count <= 0 { break }
                body.append(contentsOf: chunk[0..<count])
            }
        } else if header("Transfer-Encoding")?.lowercased() == "chunked" {
            body = readChunked(connection, initial: body)
        }

        let parts = target.split(separator: "?", maxSplits: 1, omittingEmptySubsequences: false)
        let request = ServedRequest(
            method: method, path: String(parts[0]), query: parts.count > 1 ? String(parts[1]) : nil, headers: headers,
            body: body, arrivedAt: DispatchTime.now().uptimeNanoseconds)
        // HEY has no root operation: a request to `/` is refused and not recorded.
        if request.path == "/" {
            answer(connection, ServedResponse(status: 404, headers: [("Content-Type", "text/plain")], body: Data("404 page not found".utf8)))
            return
        }
        // Recorded and numbered in one step, so two requests in flight at once never share an index.
        lock.lock()
        let index = received.count
        received.append(request)
        lock.unlock()
        var response = respond(index, request)
        if response.delay > .zero {
            Thread.sleep(forTimeInterval: Double(response.delay.components.seconds) + Double(response.delay.components.attoseconds) / 1e18)
        }
        if !response.headers.contains(where: { $0.0.caseInsensitiveCompare("Content-Type") == .orderedSame }) {
            response.headers.append(("Content-Type", "application/json"))
        }
        answer(connection, response)
    }

    private func readChunked(_ connection: Int32, initial: Data) -> Data {
        var raw = initial
        var chunk = [UInt8](repeating: 0, count: 65_536)
        while raw.range(of: Data("0\r\n\r\n".utf8)) == nil {
            let count = recv(connection, &chunk, chunk.count, 0)
            if count <= 0 { break }
            raw.append(contentsOf: chunk[0..<count])
        }
        var body = Data()
        var index = raw.startIndex
        while let lineEnd = raw[index...].range(of: Data("\r\n".utf8)),
              let sizeText = String(data: raw[index..<lineEnd.lowerBound], encoding: .utf8),
              let size = Int(sizeText.split(separator: ";")[0], radix: 16), size > 0
        {
            let start = lineEnd.upperBound
            let stop = raw.index(start, offsetBy: size, limitedBy: raw.endIndex) ?? raw.endIndex
            body.append(raw[start..<stop])
            index = raw.index(stop, offsetBy: 2, limitedBy: raw.endIndex) ?? raw.endIndex
        }
        return body
    }

    private func answer(_ connection: Int32, _ response: ServedResponse) {
        let reason = HTTPURLResponse.localizedString(forStatusCode: response.status)
        var head = "HTTP/1.1 \(response.status) \(reason.isEmpty ? "Status" : reason.capitalized)\r\n"
        for (name, value) in response.headers { head += "\(name): \(value)\r\n" }
        let bodiless = response.status == 204 || response.status == 304 || (100..<200).contains(response.status)
        if !bodiless { head += "Content-Length: \(response.body.count)\r\n" }
        head += "Connection: close\r\n\r\n"
        write(connection, Data(head.utf8))
        if !bodiless { write(connection, response.body) }
    }

    private func write(_ connection: Int32, _ data: Data) {
        data.withUnsafeBytes { raw in
            guard let base = raw.baseAddress else { return }
            var offset = 0
            while offset < data.count {
                #if canImport(Glibc)
                let sent = send(connection, base + offset, data.count - offset, Int32(MSG_NOSIGNAL))
                #else
                let sent = send(connection, base + offset, data.count - offset, 0)
                #endif
                if sent <= 0 { return }
                offset += sent
            }
        }
    }
}

struct LoopbackError: Error, CustomStringConvertible {
    let description: String
    init(_ description: String) { self.description = description }
}
