import Foundation

// SHA-256 and MD5, written out here because CryptoKit is Apple's alone and the SDK builds on
// Linux too. SHA-256 keys the response cache; MD5 is the checksum Active Storage wants a blob
// reserved with. Neither is used to keep a secret.

/// The SHA-256 of `data`, as lowercase hex.
func sha256Hex(_ data: Data) -> String {
    sha256(data).map { String(format: "%02x", $0) }.joined()
}

private let sha256K: [UInt32] = [
    0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
    0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
    0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
    0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
    0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
    0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
    0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
    0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2,
]

/// The SHA-256 of `data`.
func sha256(_ data: Data) -> [UInt8] {
    var h: [UInt32] = [0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19]
    var message = [UInt8](data)
    let bitLength = UInt64(message.count) * 8
    message.append(0x80)
    while message.count % 64 != 56 { message.append(0) }
    for shift in stride(from: 56, through: 0, by: -8) { message.append(UInt8(truncatingIfNeeded: bitLength >> UInt64(shift))) }

    var w = [UInt32](repeating: 0, count: 64)
    for chunk in stride(from: 0, to: message.count, by: 64) {
        for i in 0..<16 {
            let b = chunk + i * 4
            w[i] = UInt32(message[b]) << 24 | UInt32(message[b + 1]) << 16 | UInt32(message[b + 2]) << 8 | UInt32(message[b + 3])
        }
        for i in 16..<64 {
            let s0 = rotateRight(w[i - 15], 7) ^ rotateRight(w[i - 15], 18) ^ (w[i - 15] >> 3)
            let s1 = rotateRight(w[i - 2], 17) ^ rotateRight(w[i - 2], 19) ^ (w[i - 2] >> 10)
            w[i] = w[i - 16] &+ s0 &+ w[i - 7] &+ s1
        }
        var (a, b, c, d, e, f, g, hh) = (h[0], h[1], h[2], h[3], h[4], h[5], h[6], h[7])
        for i in 0..<64 {
            let s0 = rotateRight(e, 6) ^ rotateRight(e, 11) ^ rotateRight(e, 25)
            let t1 = hh &+ s0 &+ ((e & f) ^ (~e & g)) &+ sha256K[i] &+ w[i]
            let s1 = rotateRight(a, 2) ^ rotateRight(a, 13) ^ rotateRight(a, 22)
            let t2 = s1 &+ ((a & b) ^ (a & c) ^ (b & c))
            hh = g; g = f; f = e; e = d &+ t1; d = c; c = b; b = a; a = t1 &+ t2
        }
        h[0] = h[0] &+ a; h[1] = h[1] &+ b; h[2] = h[2] &+ c; h[3] = h[3] &+ d
        h[4] = h[4] &+ e; h[5] = h[5] &+ f; h[6] = h[6] &+ g; h[7] = h[7] &+ hh
    }
    return h.flatMap { word in (0..<4).map { UInt8(truncatingIfNeeded: word >> UInt32(24 - $0 * 8)) } }
}

private func rotateRight(_ value: UInt32, _ count: UInt32) -> UInt32 {
    (value >> count) | (value << (32 - count))
}

private let md5Shifts: [UInt32] = [
    7, 12, 17, 22, 7, 12, 17, 22, 7, 12, 17, 22, 7, 12, 17, 22,
    5, 9, 14, 20, 5, 9, 14, 20, 5, 9, 14, 20, 5, 9, 14, 20,
    4, 11, 16, 23, 4, 11, 16, 23, 4, 11, 16, 23, 4, 11, 16, 23,
    6, 10, 15, 21, 6, 10, 15, 21, 6, 10, 15, 21, 6, 10, 15, 21,
]

private let md5K: [UInt32] = (0..<64).map { UInt32(truncatingIfNeeded: Int64(abs(sin(Double($0 + 1))) * 4_294_967_296)) }

/// The MD5 of `data`.
func md5(_ data: Data) -> [UInt8] {
    var a0: UInt32 = 0x67452301
    var b0: UInt32 = 0xefcdab89
    var c0: UInt32 = 0x98badcfe
    var d0: UInt32 = 0x10325476
    var message = [UInt8](data)
    let bitLength = UInt64(message.count) * 8
    message.append(0x80)
    while message.count % 64 != 56 { message.append(0) }
    for shift in stride(from: 0, through: 56, by: 8) { message.append(UInt8(truncatingIfNeeded: bitLength >> UInt64(shift))) }

    var m = [UInt32](repeating: 0, count: 16)
    for chunk in stride(from: 0, to: message.count, by: 64) {
        for i in 0..<16 {
            let b = chunk + i * 4
            m[i] = UInt32(message[b]) | UInt32(message[b + 1]) << 8 | UInt32(message[b + 2]) << 16 | UInt32(message[b + 3]) << 24
        }
        var (a, b, c, d) = (a0, b0, c0, d0)
        for i in 0..<64 {
            var f: UInt32
            let g: Int
            switch i {
            case 0..<16: f = (b & c) | (~b & d); g = i
            case 16..<32: f = (d & b) | (~d & c); g = (5 * i + 1) % 16
            case 32..<48: f = b ^ c ^ d; g = (3 * i + 5) % 16
            default: f = c ^ (b | ~d); g = (7 * i) % 16
            }
            f = f &+ a &+ md5K[i] &+ m[g]
            a = d; d = c; c = b
            b = b &+ ((f << md5Shifts[i]) | (f >> (32 - md5Shifts[i])))
        }
        a0 = a0 &+ a; b0 = b0 &+ b; c0 = c0 &+ c; d0 = d0 &+ d
    }
    return [a0, b0, c0, d0].flatMap { word in (0..<4).map { UInt8(truncatingIfNeeded: word >> UInt32($0 * 8)) } }
}
