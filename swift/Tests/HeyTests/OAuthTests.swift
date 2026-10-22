import Foundation
import XCTest

@testable import Hey

/// A strategy that counts how often it was asked to sign, so a test can tell an unsigned route
/// never consulted it, rather than only that its header did not arrive.
final class ConsultedAuth: AuthStrategy, @unchecked Sendable {
    private let lock = NSLock()
    private var signings = 0
    private var refreshCount = 0

    var authentications: Int { lock.withLock { signings } }
    var refreshes: Int { lock.withLock { refreshCount } }

    func authenticate(_ request: inout HTTPRequest) async throws {
        lock.withLock { signings += 1 }
        request.headers.set("Authorization", "Bearer held-token")
        request.headers.set("DPoP", "held-proof")
    }

    func refresh() async throws -> Bool {
        lock.withLock { refreshCount += 1 }
        return true
    }
}

/// A transport in front of the mock that records the bound the client asked each answer's body to
/// be read to, nil being "not read at all".
final class BodyLimitProbe: Transport, @unchecked Sendable {
    private let inner: MockHey
    private let lock = NSLock()
    private var asked: [Int?] = []

    init(_ inner: MockHey) {
        self.inner = inner
    }

    var limits: [Int?] { lock.withLock { asked } }

    func send(_ request: HTTPRequest, bodyLimit: @escaping @Sendable (Int, HTTPHeaders) -> Int?) async throws -> HTTPResponse {
        try await inner.send(request) { [self] status, headers in
            let limit = bodyLimit(status, headers)
            lock.withLock { asked.append(limit) }
            return limit
        }
    }
}

final class OAuthTests: XCTestCase {
    private let codeGrant = CreateTokenRequestContent(
        grantType: "authorization_code", clientId: "client", installId: "install",
        code: "the-code", redirectUri: "heyapp://oauth/callback", codeVerifier: "the-verifier")
    private let refreshGrant = CreateTokenRequestContent(
        grantType: "refresh_token", clientId: "client", installId: "install", refreshToken: "dead-refresh")
    private let revocation = RevokeTokenRequestContent(clientId: "client", token: "old-access")
    private let tokenJSON = #"{"access_token":"new-access","token_type":"bearer","expires_in":1209600,"refresh_token":"new-refresh"}"#

    // MARK: - The generated methods

    func testCreateTokenPostsTheGrantAsJSONToTheTokenEndpoint() async throws {
        let hey = mockHey(ok(tokenJSON))
        let token = try await hey.client().oauth.createToken(body: codeGrant)

        XCTAssertEqual(hey.requests.count, 1)
        let request = hey.requests[0]
        XCTAssertEqual(request.method, "POST")
        XCTAssertEqual(request.path, "/oauth/tokens.json", "the path's .json is not doubled")
        XCTAssertNil(request.encodedQuery)
        XCTAssertEqual(request.header("Content-Type"), "application/json")
        let body = try jsonObject(request.body)
        XCTAssertEqual(body["grant_type"] as? String, "authorization_code")
        XCTAssertEqual(body["client_id"] as? String, "client")
        XCTAssertEqual(body["install_id"] as? String, "install")
        XCTAssertEqual(body["code"] as? String, "the-code")
        XCTAssertEqual(body["redirect_uri"] as? String, "heyapp://oauth/callback")
        XCTAssertEqual(body["code_verifier"] as? String, "the-verifier")
        XCTAssertNil(body["refresh_token"], "a member left out stays off the wire")

        XCTAssertEqual(token.accessToken.expose(), "new-access")
        XCTAssertEqual(token.tokenType, "bearer")
        XCTAssertEqual(token.expiresIn, 1_209_600)
        XCTAssertEqual(token.refreshToken?.expose(), "new-refresh")
        XCTAssertFalse("\(token)".contains("new-access"), "a token prints redacted")
        XCTAssertFalse("\(codeGrant)".contains("the-verifier"), "and so does a grant's secret")
    }

    func testRevokeTokenSendsTheTokenInAJSONBodyAndNeverInTheQuery() async throws {
        let hey = mockHey(ok("{}"))
        try await hey.client().oauth.revokeToken(body: revocation)

        XCTAssertEqual(hey.requests.count, 1)
        let request = hey.requests[0]
        XCTAssertEqual(request.method, "DELETE")
        XCTAssertEqual(request.path, "/oauth/tokens.json")
        XCTAssertNil(request.encodedQuery)
        XCTAssertEqual(request.header("Content-Type"), "application/json")
        let body = try jsonObject(request.body)
        XCTAssertEqual(body["client_id"] as? String, "client")
        XCTAssertEqual(body["token"] as? String, "old-access")
    }

    // MARK: - Unsigned

    func testTheTokenEndpointGoesOutWithoutTheClientsCredentialsAndWithoutAskingForThem() async throws {
        let auth = ConsultedAuth()
        let hey = mockHey(ok(tokenJSON), ok("{}"))
        let client = try hey.client(auth: auth)
        _ = try await client.oauth.createToken(body: codeGrant)
        try await client.oauth.revokeToken(body: revocation)

        XCTAssertEqual(hey.requests.count, 2)
        for request in hey.requests {
            XCTAssertNil(request.header("Authorization"), request.method)
            XCTAssertNil(request.header("DPoP"), request.method)
        }
        XCTAssertEqual(auth.authentications, 0, "the strategy is not consulted at all")
    }

    func testABearerClientSendsTheTokenEndpointWithoutItsToken() async throws {
        let hey = mockHey(ok(tokenJSON))
        _ = try await hey.client().oauth.createToken(body: codeGrant)

        XCTAssertNil(hey.requests[0].header("Authorization"))
    }

    func testA401FromTheTokenEndpointIsTheAnswerAndRefreshesNothing() async throws {
        let auth = ConsultedAuth()
        let grant = mockHey(status(401, #"{"error":"invalid_grant"}"#), ok(tokenJSON))
        let error = await assertThrows(HeyError.codeAuth, try await grant.client(auth: auth).oauth.createToken(body: refreshGrant))
        XCTAssertEqual(error?.httpStatus, 401)
        XCTAssertEqual(grant.requests.count, 1, "sent once")

        let revoke = mockHey(status(401, #"{"error":"invalid_client"}"#), ok("{}"))
        await assertThrows(HeyError.codeAuth, try await revoke.client(auth: auth).oauth.revokeToken(body: revocation))
        XCTAssertEqual(revoke.requests.count, 1, "sent once")

        XCTAssertEqual(auth.refreshes, 0, "the refresh is never asked for")
    }

    // MARK: - No redirect

    func testARedirectFromTheTokenEndpointIsAnErrorWithItsStatusAndGoesNowhere() async throws {
        for code in [301, 302, 303, 307, 308] {
            let hey = mockHey(
                status(code, #"{"error":"read from the redirect"}"#, [("Location", "/elsewhere/oauth/tokens.json")]),
                ok(tokenJSON))
            let error = await assertThrowsHeyError(try await hey.client().oauth.createToken(body: codeGrant))
            XCTAssertEqual(error?.httpStatus, code)
            XCTAssertEqual(error?.isRetryable, false, "\(code)")
            XCTAssertTrue(error?.message.contains("follows no redirect") ?? false, "\(String(describing: error))")
            XCTAssertEqual(hey.requests.map(\.path), ["/oauth/tokens.json"], "nothing goes to the Location after a \(code)")
        }
    }

    func testARedirectsBodyIsNotReadOnARouteThatFollowsNone() async throws {
        let probe = BodyLimitProbe(mockHey(status(307, #"{"error":"x"}"#, [("Location", "/elsewhere/oauth/tokens.json")])))
        await assertThrowsHeyError(try await HeyClient(accessToken: "t", transport: probe).oauth.createToken(body: codeGrant))
        XCTAssertEqual(probe.limits, [nil], "the transport is told not to read it")

        let refused = BodyLimitProbe(mockHey(status(400, #"{"error":"invalid_request"}"#)))
        await assertThrowsHeyError(try await HeyClient(accessToken: "t", transport: refused).oauth.createToken(body: codeGrant))
        XCTAssertNotNil(refused.limits.first ?? nil, "while a refusal's body is read for its message")
    }

    func testARevocationFollowsNoRedirectToAnotherOriginEither() async throws {
        let hey = mockHey(status(302, nil, [("Location", "https://elsewhere.example.com/oauth/tokens.json")]), ok("{}"))
        let error = await assertThrowsHeyError(try await hey.client().oauth.revokeToken(body: revocation))

        XCTAssertEqual(error?.httpStatus, 302)
        XCTAssertEqual(hey.requests.count, 1)
        XCTAssertEqual(hey.requests[0].url.host, "app.hey.com")
    }

    // MARK: - Driven by the route

    func testAnyRouteTheModelMarksIsSentUnsignedAndFollowsNoRedirect() async throws {
        // A route that is no token endpoint: the flags, not the path, decide.
        var route = Routes.listBoxes
        route.unsigned = true
        route.noRedirect = true
        let auth = ConsultedAuth()
        let hey = mockHey(status(401), status(302, nil, [("Location", "/boxes/all.json")]), ok("[]"))
        let client = try hey.client(auth: auth)

        await assertThrows(HeyError.codeAuth, try await client.sendVoid(try client.operation(route, [])))
        let redirect = await assertThrowsHeyError(try await client.sendVoid(try client.operation(route, [])))
        XCTAssertEqual(redirect?.httpStatus, 302)
        XCTAssertEqual(hey.requests.count, 2)
        XCTAssertTrue(hey.requests.allSatisfy { $0.header("Authorization") == nil })
        XCTAssertEqual(auth.authentications, 0)
        XCTAssertEqual(auth.refreshes, 0)
    }

    func testTheSameRouteUnmarkedIsSignedRefreshedAndFollowsItsRedirect() async throws {
        let auth = ConsultedAuth()
        let hey = mockHey(status(401), status(302, nil, [("Location", "/boxes/all.json")]), ok("[]"))
        _ = try await hey.client(auth: auth).boxes.list()

        XCTAssertEqual(hey.requests.map(\.path), ["/boxes.json", "/boxes.json", "/boxes/all.json"])
        XCTAssertTrue(hey.requests.allSatisfy { $0.header("Authorization") == "Bearer held-token" })
        XCTAssertEqual(auth.refreshes, 1)
    }

    func testAnAccountScopedClientSendsTheTokenEndpointUnscoped() async throws {
        let hey = mockHey(ok(identityJSON), ok(tokenJSON))
        let work = try await hey.client().forAccount(42)
        _ = try await work.oauth.createToken(body: codeGrant)

        XCTAssertEqual(hey.requests[1].path, "/oauth/tokens.json")
        XCTAssertNil(hey.requests[1].encodedQuery, "the account filter is HEY's API's, not the token endpoint's")
    }

    func testOnlyTheTokenEndpointIsUnsignedOrFollowsNoRedirect() {
        XCTAssertEqual(Routes.all.filter(\.unsigned).map(\.id), ["CreateToken", "RevokeToken"])
        XCTAssertEqual(Routes.all.filter(\.noRedirect).map(\.id), ["CreateToken", "RevokeToken"])
        XCTAssertEqual(Routes.createToken.service, "OAuth")
        XCTAssertEqual(Routes.revokeToken.resourceType, "token")
    }

    // MARK: - A client without credentials

    func testANoCredentialsClientSendsTheTokenEndpoint() async throws {
        let hey = mockHey(ok(tokenJSON), ok("{}"))
        let client = try hey.client(auth: NoCredentials())
        let token = try await client.oauth.createToken(body: codeGrant)
        try await client.oauth.revokeToken(body: revocation)

        XCTAssertEqual(token.accessToken.expose(), "new-access")
        XCTAssertEqual(hey.requests.map(\.method), ["POST", "DELETE"])
    }

    func testANoCredentialsClientRefusesASignedOperationWithoutSendingIt() async throws {
        let transcript = Transcript()
        let hey = mockHey(ok("[]"))
        let client = try hey.client(auth: NoCredentials(), hooks: transcript)
        let error = await assertThrows(HeyError.codeAuth, try await client.boxes.list())

        XCTAssertTrue(error?.message.contains("no credentials") ?? false, "\(String(describing: error))")
        XCTAssertEqual(hey.requests.count, 0)
        XCTAssertFalse(transcript.log.contains { $0.hasPrefix("request:") }, "\(transcript.log)")
    }

    func testANoCredentialsClientIsMadeThroughThePublicInitializer() throws {
        XCTAssertNoThrow(try HeyClient(auth: NoCredentials()))
    }
}
