import Foundation
import XCTest

@testable import Hey

final class AttachmentsTests: XCTestCase {
    private func directUpload(
        _ url: String,
        headers: String = #"{"Content-Type":"application/pdf","Content-MD5":"XUFAKrxLKna5cZ2REBfFkg==","Content-Disposition":"inline; filename=\"report.pdf\"","Authorization":"stale"}"#
    ) -> String {
        #"{"signed_id":"signed-123","attachable_sgid":"sgid-456","direct_upload":{"url":"\#(url)","headers":\#(headers)}}"#
    }

    func testAnUploadReservesABlobAndPutsTheBytesWhereHEYSaid() async throws {
        let hey = mockHey(ok(directUpload("https://storage.example.com/blobs/abc?signature=secret")), ok(""))
        let transcript = GroupAHooksLog()
        let client = try hey.client(hooks: transcript)
        let upload = try await client.attachments.upload(filename: "report.pdf", contentType: "application/pdf", content: Data("hello".utf8))
        XCTAssertEqual(upload.signedId, "signed-123")
        XCTAssertEqual(upload.attachableSgid, "sgid-456")

        let reservation = hey.requests[0]
        XCTAssertEqual(reservation.method, "POST")
        XCTAssertEqual(reservation.path, "/rails/active_storage/direct_uploads.json")
        let blob = try groupAMember(reservation.body, "blob")
        XCTAssertEqual(blob["filename"] as? String, "report.pdf")
        XCTAssertEqual(blob["byte_size"] as? Int, 5)
        XCTAssertEqual(blob["checksum"] as? String, "XUFAKrxLKna5cZ2REBfFkg==", "the MD5 of the bytes, base64 as Active Storage wants it")
        XCTAssertEqual(blob["content_type"] as? String, "application/pdf")

        let stored = hey.requests[1]
        XCTAssertEqual(stored.method, "PUT")
        XCTAssertEqual(stored.url.host, "storage.example.com")
        XCTAssertEqual(stored.path, "/blobs/abc")
        XCTAssertEqual(stored.query("signature"), "secret")
        XCTAssertEqual(stored.body, "hello")
        XCTAssertEqual(stored.header("Content-Type"), "application/pdf")
        XCTAssertEqual(stored.header("Content-MD5"), "XUFAKrxLKna5cZ2REBfFkg==")
        XCTAssertEqual(stored.header("Accept"), "*/*")
        XCTAssertNil(stored.header("Authorization"), "HEY's credentials stay on HEY, and so does the stale one it echoed")
        XCTAssertEqual(reservation.header("Authorization"), "Bearer test-token")
        XCTAssertEqual(
            transcript.operations.map(\.operation), ["CreateDirectUpload"], "the reservation is the operation; the bytes are its second request")
        XCTAssertEqual(transcript.requests[1], "PUT https://storage.example.com", "a URL that signs itself is heard as its origin alone")
    }

    func testAStorageHopOnHEYsOwnOriginStaysUnscopedAndUnsigned() async throws {
        // Storage can live on HEY's origin, and a 307 there is still the storage service's: the
        // URL it names authenticates itself, so neither hop gets the account scope or HEY's
        // credentials, and the bytes go again as they went the first time.
        let hey = mockHey(
            ok(identityJSON),
            ok(directUpload("https://app.hey.com/storage/blobs/abc?signature=secret")),
            status(307, nil, [("Location", "/storage/blobs/abc-moved?signature=secret2")]),
            ok(""))
        let client = try await hey.client().forAccount(42)
        _ = try await client.attachments.upload(filename: "report.pdf", contentType: "application/pdf", content: Data("hello".utf8))
        XCTAssertEqual(hey.requests.count, 4)
        XCTAssertEqual(hey.requests[1].query("filtered_account_id"), "42", "the reservation is HEY's, and scoped")
        for index in 2...3 {
            let put = hey.requests[index]
            XCTAssertEqual(put.method, "PUT", "hop \(index)")
            XCTAssertEqual(put.body, "hello", "hop \(index) keeps the bytes")
            XCTAssertNil(put.query("filtered_account_id"), "hop \(index) carries no account scope")
            XCTAssertNil(put.header("Authorization"), "hop \(index) carries no HEY credentials")
        }
        XCTAssertEqual(hey.requests[3].path, "/storage/blobs/abc-moved")
        XCTAssertEqual(hey.requests[3].query("signature"), "secret2", "the hop goes exactly where storage said")
        XCTAssertEqual(hey.requests[3].header("Content-Type"), "application/pdf", "with the headers storage named")
        XCTAssertEqual(hey.requests[3].header("Content-MD5"), "XUFAKrxLKna5cZ2REBfFkg==", "the checksum included, since the bytes go again")
        XCTAssertEqual(hey.requests[3].header("Content-Disposition"), #"inline; filename="report.pdf""#, "and the disposition HEY named")
    }

    func testAStorageHopThatDropsTheBytesDropsWhatDescribedThem() async throws {
        // A 303 says fetch the answer: the PUT becomes a bodyless GET, and the type, length and
        // checksum that described the bytes go with them, or the destination would be asked to
        // check a checksum against nothing. The hop is still unsigned and unscoped.
        let hey = mockHey(
            ok(identityJSON),
            ok(directUpload("https://app.hey.com/storage/blobs/abc?signature=secret")),
            status(303, nil, [("Location", "/storage/blobs/abc/status?signature=secret3")]),
            ok(""))
        _ = try await hey.client().forAccount(42).attachments.upload(filename: "report.pdf", contentType: "application/pdf", content: Data("hello".utf8))
        XCTAssertEqual(hey.requests.count, 4)
        let put = hey.requests[2]
        XCTAssertEqual(put.method, "PUT")
        XCTAssertEqual(put.header("Content-MD5"), "XUFAKrxLKna5cZ2REBfFkg==")
        let fetched = hey.requests[3]
        XCTAssertEqual(fetched.method, "GET")
        XCTAssertEqual(fetched.body, "")
        XCTAssertEqual(fetched.path, "/storage/blobs/abc/status")
        XCTAssertEqual(fetched.query("signature"), "secret3", "exactly the URL storage signed")
        XCTAssertNil(fetched.query("filtered_account_id"))
        XCTAssertNil(fetched.header("Authorization"))
        for name in ["Content-Type", "Content-Length", "Content-MD5", "Content-Disposition"] {
            XCTAssertNil(fetched.header(name), "\(name) described bytes the hop no longer carries")
        }
    }

    func testAStorageOnHEYsOwnOriginIsPutToAsBuiltWithoutTheAccountScope() async throws {
        let hey = mockHey(ok(identityJSON), ok(directUpload("https://app.hey.com/rails/active_storage/disk/token123")), ok(""))
        let work = try await hey.client().forAccount(42)
        _ = try await work.attachments.upload(filename: "a.txt", contentType: "text/plain", content: Data("x".utf8))
        XCTAssertEqual(hey.requests[1].query("filtered_account_id"), "42", "the reservation is HEY's and is scoped")
        XCTAssertNil(hey.requests[2].query("filtered_account_id"), "the put is the storage service's and is not")
        XCTAssertNil(hey.requests[2].header("Authorization"))
    }

    func testAStorageThatRefusesOrRedirectsGetsNoCredentialsEitherWay() async throws {
        let credentials = ScriptedProvider(token: "token") { _, _ in true }
        let refusing = mockHey(ok(directUpload("https://storage.example.com/blobs/abc")), status(401))
        let error = await assertThrows(
            HeyError.codeAuth,
            try await refusing.client(auth: BearerAuth(tokenProvider: credentials)).attachments.upload(
                filename: "a.txt", contentType: "text/plain", content: Data("x".utf8)))
        XCTAssertEqual(error?.httpStatus, 401)
        XCTAssertEqual(credentials.refreshes, 0, "a 401 from storage rejected none of HEY's credentials")
        XCTAssertEqual(refusing.requests.count, 2, "and nothing is resent")

        let redirecting = mockHey(
            ok(directUpload("https://storage.example.com/blobs/abc")),
            status(307, nil, [("Location", "https://storage.example.com/blobs/abc-moved")]),
            ok(""))
        _ = try await redirecting.client(auth: BearerAuth(tokenProvider: credentials)).attachments.upload(
            filename: "a.txt", contentType: "text/plain", content: Data("x".utf8))
        XCTAssertEqual(redirecting.requests.count, 3)
        XCTAssertEqual(redirecting.requests[2].path, "/blobs/abc-moved")
        XCTAssertNil(redirecting.requests[2].header("Authorization"), "a hop on storage's own origin is not signed: the put never was")
        XCTAssertEqual(redirecting.requests[2].body, "x", "a 307 keeps the bytes")
    }

    func testAnAttachmentWithNoContentTypeIsAStreamOfBytesAndOneWithoutAFilenameIsRefused() async throws {
        let hey = mockHey(ok(directUpload("https://storage.example.com/blobs/abc", headers: #"{"Content-Type":"application/octet-stream"}"#)), ok(""))
        let client = try hey.client()
        _ = try await client.attachments.upload(filename: "notes.bin", content: Data())
        XCTAssertEqual(try groupAMember(hey.requests[0].body, "blob")["content_type"] as? String, "application/octet-stream")
        XCTAssertEqual(hey.requests[1].body, "", "empty content is an empty attachment, not a mistake")
        await assertThrows(HeyError.codeUsage, try await client.attachments.upload(filename: "", contentType: "text/plain", content: Data("x".utf8)))
        XCTAssertEqual(hey.requests.count, 2)
    }

    func testAnUploadThatCannotBeMadeSafelyIsRefusedBeforeTheBytesGo() async throws {
        let empty = mockHey(ok(#"{"signed_id":"","attachable_sgid":"","direct_upload":{"url":""}}"#))
        let refused = await assertThrows(HeyError.codeAPI, try await empty.client().attachments.upload(filename: "a.txt", content: Data("x".utf8)))
        XCTAssertEqual(refused?.message, "HEY returned an empty attachment upload response")
        XCTAssertEqual(empty.requests.count, 1)

        let insecure = mockHey(ok(directUpload("http://storage.example.com/blobs/abc")))
        let unsafe = await assertThrows(HeyError.codeUsage, try await insecure.client().attachments.upload(filename: "a.txt", content: Data("x".utf8)))
        XCTAssertTrue(unsafe?.message.hasPrefix("unsafe attachment upload target") ?? false, unsafe?.message ?? "")
        XCTAssertEqual(insecure.requests.count, 1)

        let storage = mockHey(
            ok(directUpload("https://storage.example.com/blobs/abc")), status(403, "<Error>denied</Error>", [("Content-Type", "application/xml")]))
        let denied = await assertThrows(HeyError.codeForbidden, try await storage.client().attachments.upload(filename: "a.txt", content: Data("x".utf8)))
        XCTAssertEqual(denied?.httpStatus, 403)
        XCTAssertEqual(storage.requests.count, 2, "the bytes go once")
    }

    func testAStorageHeaderThatCannotBeSentIsRefusedWithoutItsValue() async throws {
        for headers in [#"{"X-Bad\nName":"v"}"#, #"{"X-Token":"secret\r\nInjected: yes"}"#] {
            let hey = mockHey(ok(directUpload("https://storage.example.com/blobs/abc", headers: headers)))
            let error = await assertThrows(HeyError.codeAPI, try await hey.client().attachments.upload(filename: "a.txt", content: Data("x".utf8)))
            XCTAssertFalse(error?.message.contains("secret") ?? true, error?.message ?? "")
            XCTAssertEqual(hey.requests.count, 1, "the bytes never go")
        }
    }

    func testAnUploadEndsItsOperationOnlyOnceTheBytesAreStored() async throws {
        let hey = mockHey(
            ok(#"{"signed_id":"s","attachable_sgid":"g","direct_upload":{"url":"https://storage.example.com/blobs/abc","headers":{"Content-Type":"text/plain"}}}"#),
            status(403, #"{"error":"no"}"#))
        let transcript = Transcript()
        let client = try hey.client(hooks: transcript)
        let error = await assertThrows(
            HeyError.codeForbidden, try await client.attachments.upload(filename: "a.txt", contentType: "text/plain", content: Data("x".utf8)))
        XCTAssertEqual(error?.httpStatus, 403)
        XCTAssertEqual(transcript.log.filter { $0.hasPrefix("start:") }.count, 1)
        XCTAssertEqual(
            transcript.log.first, "start:Attachments.CreateDirectUpload:attachment:true:nil", "the operation is the reservation's, as the model describes it")
        XCTAssertEqual(transcript.log.filter { $0.hasPrefix("response:") }, ["response:200:nil", "response:403:forbidden"])
        XCTAssertEqual(transcript.log.last, "end:CreateDirectUpload:forbidden", "the operation ends after the put, with the put's failure")
    }
}
