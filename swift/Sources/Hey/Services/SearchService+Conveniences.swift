/// An advanced search. ``query`` is the free-text part; the rest map onto the `refine[...]`
/// parameters the advanced search form submits. An empty refinement is left off the wire.
public struct SearchParams: Sendable, Equatable {
    /// The words to search for.
    public var query: String
    /// The 1-based results page. Zero and one both ask for the first.
    public var page: Int
    /// Words that must all appear.
    public var required: String
    /// Words of which at least one must appear.
    public var any: String
    /// Words that must not appear.
    public var none: String
    /// Text that must appear verbatim.
    public var exactPhrase: String
    /// Narrows by sender.
    public var from: String
    /// Narrows by recipient.
    public var to: String
    /// Narrows by subject line.
    public var subject: String
    /// `last_7_days`, `last_30_days`, `last_90_days` or a four-digit year.
    public var date: String
    /// Narrows to a box: `imbox`, `feed`, `papertrail` or `trash`.
    public var inBox: String
    /// Narrows to a folder name.
    public var label: String
    /// Narrows by attachment kind, or `any`.
    public var attachment: String

    /// A search from its parts; every part left out is left off the wire.
    public init(
        query: String = "", page: Int = 0, required: String = "", any: String = "", none: String = "",
        exactPhrase: String = "", from: String = "", to: String = "", subject: String = "", date: String = "",
        inBox: String = "", label: String = "", attachment: String = ""
    ) {
        self.query = query
        self.page = page
        self.required = required
        self.any = any
        self.none = none
        self.exactPhrase = exactPhrase
        self.from = from
        self.to = to
        self.subject = subject
        self.date = date
        self.inBox = inBox
        self.label = label
        self.attachment = attachment
    }
}

/// One page of matches and the number of the page after it. Search numbers its pages rather than
/// cursoring them, so the next page is read by passing that number back as
/// ``SearchParams/page``. It is nil on the last page: HEY only sends the `Link` header while there
/// is more to read, which is how a caller walking the results is told to stop asking rather than
/// having to ask for a page that turns out empty.
public struct SearchResults: Sendable, Equatable {
    /// The matches, grouped by topic.
    public var result: AdvancedSearchResult
    /// The number of the page after this one, while there is one.
    public var nextPage: Int?

    /// Results from their parts.
    public init(result: AdvancedSearchResult, nextPage: Int? = nil) {
        self.result = result
        self.nextPage = nextPage
    }
}

/// Searching mail, on top of the generated surface (`advanced`, `getAdvancedFilters`). HEY has no
/// JSON search endpoint — `/search` and `/advanced_search` render HTML — so results are read off
/// the advanced search page; only the refine options are JSON.
extension SearchService {
    /// Runs an advanced search and answers the matching threads, grouped by topic as the search
    /// page shows them: the topic, your posting of it, and the entries that matched as summaries —
    /// read a message with ``MessagesService/get(messageId:)``.
    public func search(_ params: SearchParams) async throws -> AdvancedSearchResult {
        try await searchPage(params).result
    }

    /// Runs the same search as ``search(_:)`` and also answers which page comes next.
    public func searchPage(_ params: SearchParams) async throws -> SearchResults {
        let page = try await advanced(options: refinements(params))
        return SearchResults(result: page.value, nextPage: page.nextPage.flatMap { Int($0) })
    }

    private func refinements(_ params: SearchParams) -> AdvancedSearchOptions {
        func set(_ value: String) -> String? { value.isEmpty ? nil : value }
        return AdvancedSearchOptions(
            q: set(params.query),
            page: params.page > 1 ? String(params.page) : nil,
            refineFrom: set(params.from),
            refineTo: set(params.to),
            refineSubject: set(params.subject),
            refineExactPhrase: set(params.exactPhrase),
            refineRequired: set(params.required),
            refineAny: set(params.any),
            refineNone: set(params.none),
            refineDate: set(params.date),
            refineIn: set(params.inBox),
            refineLabel: set(params.label),
            refineAttachment: set(params.attachment))
    }
}
