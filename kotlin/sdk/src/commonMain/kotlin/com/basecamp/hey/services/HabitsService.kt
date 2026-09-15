package com.basecamp.hey.services

import com.basecamp.hey.HeyClient
import com.basecamp.hey.generated.models.HabitPayload
import com.basecamp.hey.generated.models.HabitRequestContent
import com.basecamp.hey.generated.models.Recording
import com.basecamp.hey.generated.services.HabitsService as GeneratedHabitsService

/** A habit, as its writes take it. */
data class HabitParams(
    /** What the habit is called. */
    val name: String = "",
    /** The icon HEY draws it with. */
    val icon: String = "",
    /** The color HEY draws it in. */
    val color: String = "",
    /** The days of the week the habit runs on, 0 for Sunday through 6 for Saturday. */
    val days: List<Int> = emptyList(),
)

/**
 * Habits service with the writes that take a habit in its parts, on top of the generated
 * surface (`create`, `update`, `complete`, `uncomplete`, `stop`, `resume`, `delete`). Both
 * writes answer the habit as a recording: HEY renders it as JSON on create and on update.
 * There is no redirect fallback, so a caller never sees a "success" that wrote nothing.
 */
class HabitsService(client: HeyClient) : GeneratedHabitsService(client) {
    /** Starts a new habit and answers it as a recording. */
    suspend fun createHabit(params: HabitParams): Recording = create(habitBody(params))

    /** Edits a habit and answers it as a recording. [habitId] is the recording's id, and fields left empty are kept. */
    suspend fun updateHabit(habitId: Long, params: HabitParams): Recording = update(habitId, habitBody(params))
}

/** An empty field is left off the wire, so HEY keeps what the habit had. */
internal fun habitBody(params: HabitParams): HabitRequestContent =
    HabitRequestContent(
        HabitPayload(
            name = params.name.ifEmpty { null },
            icon = params.icon.ifEmpty { null },
            color = params.color.ifEmpty { null },
            days = params.days.ifEmpty { null },
        ),
    )
