package com.basecamp.hey.services

import com.basecamp.hey.HeyClient
import com.basecamp.hey.generated.models.AdvancedSearchResult
import com.basecamp.hey.generated.services.AdvancedSearchOptions
import com.basecamp.hey.generated.services.SearchService as GeneratedSearchService

/**
 * An advanced search. [query] is the free-text part; the rest map onto the `refine[...]`
 * parameters the advanced search form submits. An empty refinement is left off the wire.
 */
data class SearchParams(
    /** The words to search for. */
    val query: String = "",
    /** The 1-based results page. Zero and one both ask for the first. */
    val page: Int = 0,
    /** Words that must all appear. */
    val required: String = "",
    /** Words of which at least one must appear. */
    val any: String = "",
    /** Words that must not appear. */
    val none: String = "",
    /** Text that must appear verbatim. */
    val exactPhrase: String = "",
    /** Narrows by sender. */
    val from: String = "",
    /** Narrows by recipient. */
    val to: String = "",
    /** Narrows by subject line. */
    val subject: String = "",
    /** `last_7_days`, `last_30_days`, `last_90_days` or a four-digit year. */
    val date: String = "",
    /** Narrows to a box: `imbox`, `feed`, `papertrail` or `trash`. */
    val inBox: String = "",
    /** Narrows to a folder name. */
    val label: String = "",
    /** Narrows by attachment kind, or `any`. */
    val attachment: String = "",
)

/**
 * One page of matches and the number of the page after it. Search numbers its pages rather
 * than cursoring them, so the next page is read by passing that number back as
 * [SearchParams.page]. It is null on the last page: HEY only sends the `Link` header while
 * there is more to read, which is how a caller walking the results is told to stop asking
 * rather than having to ask for a page that turns out empty.
 */
data class SearchResults(
    /** The matches, grouped by topic. */
    val result: AdvancedSearchResult,
    /** The number of the page after this one, while there is one. */
    val nextPage: Int? = null,
)

/**
 * Searching mail, on top of the generated surface (`advanced`, `getAdvancedFilters`). HEY
 * has no JSON search endpoint — `/search` and `/advanced_search` render HTML — so results
 * are read off the advanced search page; only the refine options are JSON.
 */
class SearchService(client: HeyClient) : GeneratedSearchService(client) {
    /**
     * Runs an advanced search and answers the matching threads, grouped by topic as the
     * search page shows them: the topic, your posting of it, and the entries that matched as
     * summaries — read a message with `MessagesService.get`.
     */
    suspend fun search(params: SearchParams): AdvancedSearchResult = searchPage(params).result

    /** Runs the same search as [search] and also answers which page comes next. */
    suspend fun searchPage(params: SearchParams): SearchResults {
        val page = advanced(refinements(params))
        return SearchResults(result = page.value, nextPage = page.nextPage?.toIntOrNull())
    }

    private fun refinements(params: SearchParams): AdvancedSearchOptions =
        AdvancedSearchOptions(
            q = params.query.ifEmpty { null },
            page = params.page.takeIf { it > 1 }?.toString(),
            refineFrom = params.from.ifEmpty { null },
            refineTo = params.to.ifEmpty { null },
            refineSubject = params.subject.ifEmpty { null },
            refineExactPhrase = params.exactPhrase.ifEmpty { null },
            refineRequired = params.required.ifEmpty { null },
            refineAny = params.any.ifEmpty { null },
            refineNone = params.none.ifEmpty { null },
            refineDate = params.date.ifEmpty { null },
            refineIn = params.inBox.ifEmpty { null },
            refineLabel = params.label.ifEmpty { null },
            refineAttachment = params.attachment.ifEmpty { null },
        )
}
