import Foundation

/// What an attachment is taken to be when the caller names no content type.
private let defaultAttachmentContentType = "application/octet-stream"

/// Uploading an outgoing attachment on top of the generated `createDirectUpload`. An upload takes
/// two requests — HEY reserves an Active Storage blob and names a storage URL, and the bytes then
/// go to that URL rather than to HEY.
extension AttachmentsService {
    /// Reserves an Active Storage blob and uploads the bytes to the storage URL HEY named. The
    /// answer's `attachableSgid` is what embeds the attachment in Trix rich text.
    ///
    /// Empty content is an empty attachment rather than a mistake, so only a missing filename is
    /// refused.
    public func upload(filename: String, contentType: String? = nil, content: Data) async throws -> DirectUpload {
        if filename.isEmpty { throw HeyError.usage(message: "an attachment needs a filename") }
        let body = CreateDirectUploadRequestContent(
            blob: DirectUploadBlob(
                filename: filename,
                byteSize: content.count,
                checksum: Data(md5(content)).base64EncodedString(),
                contentType: contentType ?? defaultAttachmentContentType))
        // Both requests go quiet inside one operation — the reservation's, as the model describes
        // it — so the hooks hear Attachments.CreateDirectUpload once and hear it end only once the
        // bytes are stored, with the failure when the storage service refuses them.
        var reservation = try client.operation(Routes.createDirectUpload, [])
        try reservation.json(body)
        reservation.quiet()
        let request = reservation
        return try await client.asOperation(reservation.info) {
            let upload = try reservedUpload(try await client.send(request, as: DirectUpload.self))
            try await storeAttachment(upload.directUpload, content)
            return upload
        }
    }

    /// Puts the bytes to the storage service. This is the one request the SDK makes outside the
    /// HEY API: the storage URL authenticates itself and takes exactly the headers HEY named —
    /// including any `Authorization` the storage service wants, which is why the HEY credentials
    /// must not ride along — so the request goes out unsigned, once, and quietly: the reservation
    /// and this are the two requests of the one operation the hooks hear.
    private func storeAttachment(_ target: DirectUploadTarget, _ content: Data) async throws {
        guard let url = parseAbsoluteURL(target.url) else {
            throw HeyError.api(
                message: "HEY named an attachment upload target that is not a URL", httpStatus: nil, retryable: false, detail: ErrorDetail())
        }
        do {
            try requireSecureEndpoint(url)
        } catch let HeyError.usage(message, _) {
            throw HeyError.usage(message: "unsafe attachment upload target: \(message)")
        }
        var operation = Operation.at(.put, url)
        operation.unsigned()
        operation.quiet()
        operation.idempotent = false
        operation.accept = "*/*"
        var contentType = defaultAttachmentContentType
        for (name, value) in try storageHeaders(target) {
            if name.caseInsensitiveCompare("Content-Type") == .orderedSame {
                contentType = value
            } else {
                operation.header(name, value)
            }
        }
        operation.bodyBytes(contentType: contentType, content)
        try await client.sendVoid(operation)
    }
}

/// The blob HEY reserved, once it carries everything the upload needs. HEY answers 200 with a
/// payload rather than a status when it has nothing to give, so the fields are what say whether
/// there is an upload to make.
private func reservedUpload(_ upload: DirectUpload) throws -> DirectUpload {
    if upload.signedId.isEmpty || upload.attachableSgid.isEmpty || upload.directUpload.url.isEmpty {
        throw HeyError.api(message: "HEY returned an empty attachment upload response", httpStatus: nil, retryable: false, detail: ErrorDetail())
    }
    return upload
}

/// The headers HEY named, by name, with any `Authorization` among them dropped: the SDK's own
/// credentials never reach the storage service, and neither does a stale one HEY echoed. A header
/// the transport could not send — a name that is not a token, a value with a line break or another
/// control character in it — is refused here, without its value.
func storageHeaders(_ target: DirectUploadTarget) throws -> [(String, String)] {
    try (target.headers ?? [:])
        .filter { $0.key.caseInsensitiveCompare("Authorization") != .orderedSame }
        .sorted { $0.key < $1.key }
        .map { name, value in
            guard isHeaderToken(name), !value.unicodeScalars.contains(where: { ($0.value < 0x20 && $0 != "\t") || $0.value == 0x7F }) else {
                throw HeyError.api(
                    message: "HEY named an attachment upload header \"\(name)\" that cannot be sent", httpStatus: nil, retryable: false,
                    detail: ErrorDetail())
            }
            return (name, value)
        }
}

/// Whether a header name is an RFC 9110 token: one or more visible ASCII characters, none of them a
/// separator.
private func isHeaderToken(_ name: String) -> Bool {
    !name.isEmpty && name.unicodeScalars.allSatisfy { scalar in
        scalar.value > 0x20 && scalar.value < 0x7F && !"\"(),/:;<=>?@[\\]{}".unicodeScalars.contains(scalar)
    }
}
