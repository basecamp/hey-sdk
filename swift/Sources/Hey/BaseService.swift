/// What every service is built on: the client it sends through. Generated services extend it,
/// and the conveniences are extensions of the generated services.
open class BaseService: @unchecked Sendable {
    /// The client this service sends through.
    public let client: HeyClient

    public init(client: HeyClient) {
        self.client = client
    }
}
