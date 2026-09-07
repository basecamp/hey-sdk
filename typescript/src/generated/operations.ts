// Generated from openapi.json and behavior-model.json. Do not edit.
import type { OperationInput, OperationResponse, RequestOptions, Transport } from '../protocol.js';

export const operationMetadata = {
  "DeleteExtenzion": {
    "method": "DELETE",
    "path": "/accounts/{accountId}/domains/extenzions/{extenzionId}",
    "parameters": [
      {
        "name": "accountId",
        "in": "path",
        "required": true,
        "type": "integer"
      },
      {
        "name": "extenzionId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "AdvancedSearch": {
    "method": "GET",
    "path": "/advanced_search.json",
    "parameters": [
      {
        "name": "q",
        "in": "query",
        "required": false,
        "type": "string"
      },
      {
        "name": "page",
        "in": "query",
        "required": false,
        "type": "string"
      },
      {
        "name": "refine[from]",
        "in": "query",
        "required": false,
        "type": "string"
      },
      {
        "name": "refine[to]",
        "in": "query",
        "required": false,
        "type": "string"
      },
      {
        "name": "refine[subject]",
        "in": "query",
        "required": false,
        "type": "string"
      },
      {
        "name": "refine[exact_phrase]",
        "in": "query",
        "required": false,
        "type": "string"
      },
      {
        "name": "refine[required]",
        "in": "query",
        "required": false,
        "type": "string"
      },
      {
        "name": "refine[any]",
        "in": "query",
        "required": false,
        "type": "string"
      },
      {
        "name": "refine[none]",
        "in": "query",
        "required": false,
        "type": "string"
      },
      {
        "name": "refine[date]",
        "in": "query",
        "required": false,
        "type": "string"
      },
      {
        "name": "refine[in]",
        "in": "query",
        "required": false,
        "type": "string"
      },
      {
        "name": "refine[label]",
        "in": "query",
        "required": false,
        "type": "string"
      },
      {
        "name": "refine[attachment]",
        "in": "query",
        "required": false,
        "type": "string"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "pagination": {
      "style": "link"
    },
    "actingSender": false,
    "actingUser": false
  },
  "GetAdvancedSearchFilters": {
    "method": "GET",
    "path": "/advanced_search_filters.json",
    "parameters": [],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "ListBoxes": {
    "method": "GET",
    "path": "/boxes.json",
    "parameters": [],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "pagination": {
      "style": "link",
      "totalCountHeader": "X-Total-Count"
    },
    "actingSender": false,
    "actingUser": false
  },
  "GetBox": {
    "method": "GET",
    "path": "/boxes/{boxId}",
    "parameters": [
      {
        "name": "boxId",
        "in": "path",
        "required": true,
        "type": "integer"
      },
      {
        "name": "page",
        "in": "query",
        "required": false,
        "type": "string"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "CreateBoxDesignation": {
    "method": "POST",
    "path": "/boxes/{boxId}/designations.json",
    "parameters": [
      {
        "name": "boxId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": true,
    "safe": false,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "DeleteBoxDesignation": {
    "method": "DELETE",
    "path": "/boxes/{boxId}/designations/{designationId}",
    "parameters": [
      {
        "name": "boxId",
        "in": "path",
        "required": true,
        "type": "integer"
      },
      {
        "name": "designationId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "ListBoxGroups": {
    "method": "GET",
    "path": "/boxes/{boxId}/groups.json",
    "parameters": [
      {
        "name": "boxId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "CreateBoxGroup": {
    "method": "POST",
    "path": "/boxes/{boxId}/groups.json",
    "parameters": [
      {
        "name": "boxId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": true,
    "safe": false,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "DeleteBoxGroup": {
    "method": "DELETE",
    "path": "/boxes/{boxId}/groups/{groupId}",
    "parameters": [
      {
        "name": "boxId",
        "in": "path",
        "required": true,
        "type": "integer"
      },
      {
        "name": "groupId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "GetBoxGroup": {
    "method": "GET",
    "path": "/boxes/{boxId}/groups/{groupId}",
    "parameters": [
      {
        "name": "boxId",
        "in": "path",
        "required": true,
        "type": "integer"
      },
      {
        "name": "groupId",
        "in": "path",
        "required": true,
        "type": "integer"
      },
      {
        "name": "page",
        "in": "query",
        "required": false,
        "type": "string"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "pagination": {
      "style": "link",
      "totalCountHeader": "X-Total-Count"
    },
    "actingSender": false,
    "actingUser": false
  },
  "MarkBoxSeen": {
    "method": "POST",
    "path": "/boxes/{boxId}/observation.json",
    "parameters": [
      {
        "name": "boxId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": false,
    "safe": false,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "GetBoxPostingChanges": {
    "method": "GET",
    "path": "/boxes/{boxId}/postings/changes.json",
    "parameters": [
      {
        "name": "boxId",
        "in": "path",
        "required": true,
        "type": "integer"
      },
      {
        "name": "since",
        "in": "query",
        "required": true,
        "type": "string"
      },
      {
        "name": "v",
        "in": "query",
        "required": false,
        "type": "string"
      },
      {
        "name": "page",
        "in": "query",
        "required": false,
        "type": "string"
      },
      {
        "name": "per_page",
        "in": "query",
        "required": false,
        "type": "string"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "pagination": {
      "style": "link",
      "totalCountHeader": "X-Total-Count"
    },
    "actingSender": false,
    "actingUser": false
  },
  "GetBubblebox": {
    "method": "GET",
    "path": "/bubble_up.json",
    "parameters": [
      {
        "name": "page",
        "in": "query",
        "required": false,
        "type": "string"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "CreateBulkReply": {
    "method": "POST",
    "path": "/bulk_replies.json",
    "parameters": [],
    "bodyRequired": true,
    "safe": false,
    "actingSender": false,
    "actingUser": false
  },
  "NewBulkReply": {
    "method": "GET",
    "path": "/bulk_replies/new.json",
    "parameters": [
      {
        "name": "posting_ids",
        "in": "query",
        "required": true,
        "type": "string"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "ListCalendarDays": {
    "method": "GET",
    "path": "/calendar/days.json",
    "parameters": [
      {
        "name": "starts_at",
        "in": "query",
        "required": false,
        "type": "string"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "GetCalendarDay": {
    "method": "GET",
    "path": "/calendar/days/{day}",
    "parameters": [
      {
        "name": "day",
        "in": "path",
        "required": true,
        "type": "string"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "UncompleteHabit": {
    "method": "DELETE",
    "path": "/calendar/days/{day}/habits/{habitId}/completions",
    "parameters": [
      {
        "name": "day",
        "in": "path",
        "required": true,
        "type": "string"
      },
      {
        "name": "habitId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "CompleteHabit": {
    "method": "POST",
    "path": "/calendar/days/{day}/habits/{habitId}/completions",
    "parameters": [
      {
        "name": "day",
        "in": "path",
        "required": true,
        "type": "string"
      },
      {
        "name": "habitId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "GetJournalEntry": {
    "method": "GET",
    "path": "/calendar/days/{day}/journal_entry",
    "parameters": [
      {
        "name": "day",
        "in": "path",
        "required": true,
        "type": "string"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "UpdateJournalEntry": {
    "method": "PATCH",
    "path": "/calendar/days/{day}/journal_entry",
    "parameters": [
      {
        "name": "day",
        "in": "path",
        "required": true,
        "type": "string"
      }
    ],
    "bodyRequired": true,
    "safe": false,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "DeleteCalendarEvent": {
    "method": "DELETE",
    "path": "/calendar/events/{eventId}",
    "parameters": [
      {
        "name": "eventId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "DeleteCalendarEventOccurrence": {
    "method": "DELETE",
    "path": "/calendar/events/{eventId}/occurrences/{occurrence}",
    "parameters": [
      {
        "name": "eventId",
        "in": "path",
        "required": true,
        "type": "integer"
      },
      {
        "name": "occurrence",
        "in": "path",
        "required": true,
        "type": "string"
      },
      {
        "name": "apply_to_future",
        "in": "query",
        "required": false,
        "type": "boolean"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "CreateHabit": {
    "method": "POST",
    "path": "/calendar/habits.json",
    "parameters": [],
    "bodyRequired": true,
    "safe": false,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "DeleteHabit": {
    "method": "DELETE",
    "path": "/calendar/habits/{habitId}",
    "parameters": [
      {
        "name": "habitId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "UpdateHabit": {
    "method": "PATCH",
    "path": "/calendar/habits/{habitId}",
    "parameters": [
      {
        "name": "habitId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": true,
    "safe": true,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "ResumeHabit": {
    "method": "DELETE",
    "path": "/calendar/habits/{habitId}/stop.json",
    "parameters": [
      {
        "name": "habitId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "StopHabit": {
    "method": "POST",
    "path": "/calendar/habits/{habitId}/stop.json",
    "parameters": [
      {
        "name": "habitId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": false,
    "safe": false,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "UpdateFirstWeekDay": {
    "method": "PUT",
    "path": "/calendar/identity/first_week_day",
    "parameters": [],
    "bodyRequired": true,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "ListJournalEntries": {
    "method": "GET",
    "path": "/calendar/journal_entries",
    "parameters": [
      {
        "name": "page",
        "in": "query",
        "required": false,
        "type": "string"
      },
      {
        "name": "q",
        "in": "query",
        "required": false,
        "type": "string"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "pagination": {
      "style": "link"
    },
    "actingSender": false,
    "actingUser": false
  },
  "GetOngoingTimeTrack": {
    "method": "GET",
    "path": "/calendar/ongoing_time_track.json",
    "parameters": [],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "emptyOn": [
      404
    ],
    "actingSender": false,
    "actingUser": false
  },
  "StartTimeTrack": {
    "method": "POST",
    "path": "/calendar/ongoing_time_track.json",
    "parameters": [],
    "bodyRequired": false,
    "safe": false,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "ListTimeTracks": {
    "method": "GET",
    "path": "/calendar/time_tracks.json",
    "parameters": [
      {
        "name": "page",
        "in": "query",
        "required": false,
        "type": "string"
      },
      {
        "name": "category_id",
        "in": "query",
        "required": false,
        "type": "integer"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "pagination": {
      "style": "link"
    },
    "actingSender": false,
    "actingUser": false
  },
  "CreateTimeTrack": {
    "method": "POST",
    "path": "/calendar/time_tracks.json",
    "parameters": [],
    "bodyRequired": true,
    "safe": false,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "ListTimeTrackCategories": {
    "method": "GET",
    "path": "/calendar/time_tracks/categories.json",
    "parameters": [],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "DeleteTimeTrack": {
    "method": "DELETE",
    "path": "/calendar/time_tracks/{timeTrackId}",
    "parameters": [
      {
        "name": "timeTrackId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "UpdateTimeTrack": {
    "method": "PUT",
    "path": "/calendar/time_tracks/{timeTrackId}",
    "parameters": [
      {
        "name": "timeTrackId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": true,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "CreateCalendarTodo": {
    "method": "POST",
    "path": "/calendar/todos.json",
    "parameters": [],
    "bodyRequired": true,
    "safe": false,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "DeleteCalendarTodo": {
    "method": "DELETE",
    "path": "/calendar/todos/{todoId}",
    "parameters": [
      {
        "name": "todoId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "UpdateCalendarTodo": {
    "method": "PATCH",
    "path": "/calendar/todos/{todoId}",
    "parameters": [
      {
        "name": "todoId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": true,
    "safe": true,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "UncompleteCalendarTodo": {
    "method": "DELETE",
    "path": "/calendar/todos/{todoId}/completions",
    "parameters": [
      {
        "name": "todoId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "CompleteCalendarTodo": {
    "method": "POST",
    "path": "/calendar/todos/{todoId}/completions",
    "parameters": [
      {
        "name": "todoId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "ListCalendarWeeks": {
    "method": "GET",
    "path": "/calendar/weeks.json",
    "parameters": [
      {
        "name": "starts_at",
        "in": "query",
        "required": false,
        "type": "string"
      },
      {
        "name": "centered_at",
        "in": "query",
        "required": false,
        "type": "string"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "GetCalendarWeek": {
    "method": "GET",
    "path": "/calendar/weeks/{week}",
    "parameters": [
      {
        "name": "week",
        "in": "path",
        "required": true,
        "type": "string"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "GetCalendarYear": {
    "method": "GET",
    "path": "/calendar/years/{year}",
    "parameters": [
      {
        "name": "year",
        "in": "path",
        "required": true,
        "type": "string"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "ListCalendars": {
    "method": "GET",
    "path": "/calendars.json",
    "parameters": [],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "GetCalendarRecordings": {
    "method": "GET",
    "path": "/calendars/{calendarId}/recordings",
    "parameters": [
      {
        "name": "calendarId",
        "in": "path",
        "required": true,
        "type": "integer"
      },
      {
        "name": "starts_on",
        "in": "query",
        "required": false,
        "type": "string"
      },
      {
        "name": "ends_on",
        "in": "query",
        "required": false,
        "type": "string"
      },
      {
        "name": "page",
        "in": "query",
        "required": false,
        "type": "string"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "pagination": {
      "style": "window"
    },
    "actingSender": false,
    "actingUser": false
  },
  "ToggleCalendar": {
    "method": "POST",
    "path": "/calendars/{calendarId}/toggle",
    "parameters": [
      {
        "name": "calendarId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": false,
    "safe": false,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "GetClearances": {
    "method": "GET",
    "path": "/clearances.json",
    "parameters": [
      {
        "name": "include_clearances",
        "in": "query",
        "required": false,
        "type": "boolean"
      },
      {
        "name": "page",
        "in": "query",
        "required": false,
        "type": "string"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "pagination": {
      "style": "link",
      "totalCountHeader": "X-Total-Count"
    },
    "actingSender": false,
    "actingUser": false
  },
  "BulkUpdateClearances": {
    "method": "PATCH",
    "path": "/clearances/bulk.json",
    "parameters": [],
    "bodyRequired": true,
    "safe": true,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "PuntClearances": {
    "method": "POST",
    "path": "/clearances/punt.json",
    "parameters": [],
    "bodyRequired": false,
    "safe": false,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "UpdateClearance": {
    "method": "PATCH",
    "path": "/clearances/{clearanceId}",
    "parameters": [
      {
        "name": "clearanceId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": true,
    "safe": true,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "ListClips": {
    "method": "GET",
    "path": "/clips.json",
    "parameters": [
      {
        "name": "page",
        "in": "query",
        "required": false,
        "type": "string"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "pagination": {
      "style": "link",
      "totalCountHeader": "X-Total-Count"
    },
    "actingSender": false,
    "actingUser": false
  },
  "ListCollections": {
    "method": "GET",
    "path": "/collections.json",
    "parameters": [],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "GetCollection": {
    "method": "GET",
    "path": "/collections/{collectionId}",
    "parameters": [
      {
        "name": "collectionId",
        "in": "path",
        "required": true,
        "type": "integer"
      },
      {
        "name": "page",
        "in": "query",
        "required": false,
        "type": "string"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "pagination": {
      "style": "link",
      "totalCountHeader": "X-Total-Count"
    },
    "actingSender": false,
    "actingUser": false
  },
  "UpdateCollection": {
    "method": "PATCH",
    "path": "/collections/{collectionId}",
    "parameters": [
      {
        "name": "collectionId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": true,
    "safe": true,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "ListContacts": {
    "method": "GET",
    "path": "/contacts.json",
    "parameters": [
      {
        "name": "page",
        "in": "query",
        "required": false,
        "type": "string"
      },
      {
        "name": "q",
        "in": "query",
        "required": false,
        "type": "string"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "pagination": {
      "style": "link",
      "totalCountHeader": "X-Total-Count"
    },
    "actingSender": false,
    "actingUser": false
  },
  "CreateContact": {
    "method": "POST",
    "path": "/contacts.json",
    "parameters": [],
    "bodyRequired": true,
    "safe": false,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": true
  },
  "HideContact": {
    "method": "DELETE",
    "path": "/contacts/{contactId}",
    "parameters": [
      {
        "name": "contactId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "GetContact": {
    "method": "GET",
    "path": "/contacts/{contactId}",
    "parameters": [
      {
        "name": "contactId",
        "in": "path",
        "required": true,
        "type": "integer"
      },
      {
        "name": "page",
        "in": "query",
        "required": false,
        "type": "string"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "pagination": {
      "style": "link",
      "totalCountHeader": "X-Total-Count"
    },
    "actingSender": false,
    "actingUser": false
  },
  "UpdateContact": {
    "method": "PATCH",
    "path": "/contacts/{contactId}",
    "parameters": [
      {
        "name": "contactId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": true,
    "safe": true,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "UnbundleContact": {
    "method": "DELETE",
    "path": "/contacts/{contactId}/bundle.json",
    "parameters": [
      {
        "name": "contactId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "BundleContact": {
    "method": "POST",
    "path": "/contacts/{contactId}/bundle.json",
    "parameters": [
      {
        "name": "contactId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": false,
    "safe": false,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "UpdateContactClearance": {
    "method": "PATCH",
    "path": "/contacts/{contactId}/clearance.json",
    "parameters": [
      {
        "name": "contactId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": true,
    "safe": true,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "DeleteContactNote": {
    "method": "DELETE",
    "path": "/contacts/{contactId}/note.json",
    "parameters": [
      {
        "name": "contactId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "GetContactNote": {
    "method": "GET",
    "path": "/contacts/{contactId}/note.json",
    "parameters": [
      {
        "name": "contactId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "UpdateContactNote": {
    "method": "PATCH",
    "path": "/contacts/{contactId}/note.json",
    "parameters": [
      {
        "name": "contactId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": true,
    "safe": true,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "RevealContact": {
    "method": "POST",
    "path": "/contacts/{contactId}/reveal.json",
    "parameters": [
      {
        "name": "contactId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": false,
    "safe": false,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "ListDrafts": {
    "method": "GET",
    "path": "/entries/drafts.json",
    "parameters": [
      {
        "name": "page",
        "in": "query",
        "required": false,
        "type": "string"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "pagination": {
      "style": "link",
      "totalCountHeader": "X-Total-Count"
    },
    "actingSender": false,
    "actingUser": false
  },
  "DeleteDraft": {
    "method": "DELETE",
    "path": "/entries/drafts/{entryId}",
    "parameters": [
      {
        "name": "entryId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "NewEntryForward": {
    "method": "GET",
    "path": "/entries/{entryId}/forwards/new.json",
    "parameters": [
      {
        "name": "entryId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "CreateReply": {
    "method": "POST",
    "path": "/entries/{entryId}/replies.json",
    "parameters": [
      {
        "name": "entryId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": true,
    "safe": false,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": true,
    "actingUser": false
  },
  "NewEntryReply": {
    "method": "GET",
    "path": "/entries/{entryId}/replies/new.json",
    "parameters": [
      {
        "name": "entryId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "MarkEntrySpam": {
    "method": "PUT",
    "path": "/entries/{entryId}/status/spam.json",
    "parameters": [
      {
        "name": "entryId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "GetFeedbox": {
    "method": "GET",
    "path": "/feedbox.json",
    "parameters": [
      {
        "name": "page",
        "in": "query",
        "required": false,
        "type": "string"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "GetFolder": {
    "method": "GET",
    "path": "/folders/{folderId}",
    "parameters": [
      {
        "name": "folderId",
        "in": "path",
        "required": true,
        "type": "integer"
      },
      {
        "name": "page",
        "in": "query",
        "required": false,
        "type": "string"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "pagination": {
      "style": "link",
      "totalCountHeader": "X-Total-Count"
    },
    "actingSender": false,
    "actingUser": false
  },
  "GetIdentity": {
    "method": "GET",
    "path": "/identity.json",
    "parameters": [],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "UpdateTimeFormat": {
    "method": "PUT",
    "path": "/identity/time_format",
    "parameters": [],
    "bodyRequired": true,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "GetImbox": {
    "method": "GET",
    "path": "/imbox.json",
    "parameters": [
      {
        "name": "page",
        "in": "query",
        "required": false,
        "type": "string"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "GetImboxSeen": {
    "method": "GET",
    "path": "/imbox/seen.json",
    "parameters": [
      {
        "name": "page",
        "in": "query",
        "required": false,
        "type": "string"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "CreateMessage": {
    "method": "POST",
    "path": "/messages.json",
    "parameters": [],
    "bodyRequired": true,
    "safe": false,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": true,
    "actingUser": false
  },
  "GetMessage": {
    "method": "GET",
    "path": "/messages/{messageId}",
    "parameters": [
      {
        "name": "messageId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "UpdateMessage": {
    "method": "PUT",
    "path": "/messages/{messageId}",
    "parameters": [
      {
        "name": "messageId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": true,
    "safe": false,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": true,
    "actingUser": false
  },
  "GetMessageEdit": {
    "method": "GET",
    "path": "/messages/{messageId}/edit.json",
    "parameters": [
      {
        "name": "messageId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "GetMyClearances": {
    "method": "GET",
    "path": "/my/clearances.json",
    "parameters": [
      {
        "name": "page",
        "in": "query",
        "required": false,
        "type": "string"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "pagination": {
      "style": "link",
      "totalCountHeader": "X-Total-Count"
    },
    "actingSender": false,
    "actingUser": false
  },
  "UpdateMyClearance": {
    "method": "PATCH",
    "path": "/my/clearances/{clearanceId}",
    "parameters": [
      {
        "name": "clearanceId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": true,
    "safe": true,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "GetNavigation": {
    "method": "GET",
    "path": "/my/navigation.json",
    "parameters": [],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "GetTrailbox": {
    "method": "GET",
    "path": "/paper_trail.json",
    "parameters": [
      {
        "name": "page",
        "in": "query",
        "required": false,
        "type": "string"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "RemovePostingsFromBoxGroup": {
    "method": "DELETE",
    "path": "/postings/box_groups.json",
    "parameters": [
      {
        "name": "posting_ids",
        "in": "query",
        "required": true,
        "type": "string"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "AddPostingsToBoxGroup": {
    "method": "POST",
    "path": "/postings/box_groups.json",
    "parameters": [],
    "bodyRequired": true,
    "safe": false,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "CancelPostingsBubbleUp": {
    "method": "DELETE",
    "path": "/postings/bubble_up.json",
    "parameters": [
      {
        "name": "posting_ids",
        "in": "query",
        "required": true,
        "type": "string"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "SchedulePostingsBubbleUp": {
    "method": "POST",
    "path": "/postings/bubble_up.json",
    "parameters": [],
    "bodyRequired": true,
    "safe": false,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "BubbleUpPostingsNow": {
    "method": "POST",
    "path": "/postings/bulk_bubble_up_now.json",
    "parameters": [],
    "bodyRequired": true,
    "safe": false,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "UnfilePostings": {
    "method": "DELETE",
    "path": "/postings/filings.json",
    "parameters": [
      {
        "name": "posting_ids",
        "in": "query",
        "required": true,
        "type": "string"
      },
      {
        "name": "folder_id",
        "in": "query",
        "required": false,
        "type": "integer"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "FilePostings": {
    "method": "POST",
    "path": "/postings/filings.json",
    "parameters": [],
    "bodyRequired": true,
    "safe": false,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "CreateFolderForPostings": {
    "method": "POST",
    "path": "/postings/folders.json",
    "parameters": [],
    "bodyRequired": true,
    "safe": false,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "MovePostings": {
    "method": "POST",
    "path": "/postings/moves.json",
    "parameters": [],
    "bodyRequired": true,
    "safe": false,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "UnmutePostings": {
    "method": "DELETE",
    "path": "/postings/mutings.json",
    "parameters": [
      {
        "name": "posting_ids",
        "in": "query",
        "required": true,
        "type": "string"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "MutePostings": {
    "method": "POST",
    "path": "/postings/mutings.json",
    "parameters": [],
    "bodyRequired": true,
    "safe": false,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "MarkPostingsSeen": {
    "method": "POST",
    "path": "/postings/seen.json",
    "parameters": [],
    "bodyRequired": true,
    "safe": false,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "MarkPostingsSpam": {
    "method": "POST",
    "path": "/postings/spam.json",
    "parameters": [],
    "bodyRequired": true,
    "safe": false,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "TrashPostings": {
    "method": "POST",
    "path": "/postings/trash.json",
    "parameters": [],
    "bodyRequired": true,
    "safe": false,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "MarkPostingsUnseen": {
    "method": "POST",
    "path": "/postings/unseen.json",
    "parameters": [],
    "bodyRequired": true,
    "safe": false,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "GetBundleUnseenPostings": {
    "method": "GET",
    "path": "/postings/{postingId}/bundles/unseen.json",
    "parameters": [
      {
        "name": "postingId",
        "in": "path",
        "required": true,
        "type": "integer"
      },
      {
        "name": "page",
        "in": "query",
        "required": false,
        "type": "string"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "pagination": {
      "style": "link",
      "totalCountHeader": "X-Total-Count"
    },
    "actingSender": false,
    "actingUser": false
  },
  "CreateDirectUpload": {
    "method": "POST",
    "path": "/rails/active_storage/direct_uploads.json",
    "parameters": [],
    "bodyRequired": true,
    "safe": false,
    "actingSender": false,
    "actingUser": false
  },
  "GetLaterbox": {
    "method": "GET",
    "path": "/reply_later.json",
    "parameters": [
      {
        "name": "page",
        "in": "query",
        "required": false,
        "type": "string"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "GetAsidebox": {
    "method": "GET",
    "path": "/set_aside.json",
    "parameters": [
      {
        "name": "page",
        "in": "query",
        "required": false,
        "type": "string"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "ListSnippets": {
    "method": "GET",
    "path": "/snippets.json",
    "parameters": [],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "ListStickies": {
    "method": "GET",
    "path": "/stickies.json",
    "parameters": [
      {
        "name": "limit",
        "in": "query",
        "required": false,
        "type": "integer"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "CreateSticky": {
    "method": "POST",
    "path": "/stickies.json",
    "parameters": [],
    "bodyRequired": true,
    "safe": false,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "MoveSticky": {
    "method": "POST",
    "path": "/stickies/moves.json",
    "parameters": [],
    "bodyRequired": true,
    "safe": false,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "DeleteSticky": {
    "method": "DELETE",
    "path": "/stickies/{stickyId}",
    "parameters": [
      {
        "name": "stickyId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "UpdateSticky": {
    "method": "PATCH",
    "path": "/stickies/{stickyId}",
    "parameters": [
      {
        "name": "stickyId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": true,
    "safe": true,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "GetEverythingTopics": {
    "method": "GET",
    "path": "/topics/everything.json",
    "parameters": [
      {
        "name": "page",
        "in": "query",
        "required": false,
        "type": "string"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "pagination": {
      "style": "link",
      "totalCountHeader": "X-Total-Count"
    },
    "actingSender": false,
    "actingUser": false
  },
  "GetSentTopics": {
    "method": "GET",
    "path": "/topics/sent.json",
    "parameters": [
      {
        "name": "page",
        "in": "query",
        "required": false,
        "type": "string"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "pagination": {
      "style": "link",
      "totalCountHeader": "X-Total-Count"
    },
    "actingSender": false,
    "actingUser": false
  },
  "GetSpamTopics": {
    "method": "GET",
    "path": "/topics/spam.json",
    "parameters": [
      {
        "name": "page",
        "in": "query",
        "required": false,
        "type": "string"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "pagination": {
      "style": "link",
      "totalCountHeader": "X-Total-Count"
    },
    "actingSender": false,
    "actingUser": false
  },
  "EmptySpam": {
    "method": "DELETE",
    "path": "/topics/spam/all.json",
    "parameters": [],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "GetTrashTopics": {
    "method": "GET",
    "path": "/topics/trash.json",
    "parameters": [
      {
        "name": "page",
        "in": "query",
        "required": false,
        "type": "string"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "pagination": {
      "style": "link",
      "totalCountHeader": "X-Total-Count"
    },
    "actingSender": false,
    "actingUser": false
  },
  "EmptyTrash": {
    "method": "DELETE",
    "path": "/topics/trash/all.json",
    "parameters": [],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "GetTopic": {
    "method": "GET",
    "path": "/topics/{topicId}",
    "parameters": [
      {
        "name": "topicId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "GetTopicEntries": {
    "method": "GET",
    "path": "/topics/{topicId}/entries",
    "parameters": [
      {
        "name": "topicId",
        "in": "path",
        "required": true,
        "type": "integer"
      },
      {
        "name": "page",
        "in": "query",
        "required": false,
        "type": "string"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "pagination": {
      "style": "link",
      "totalCountHeader": "X-Total-Count"
    },
    "actingSender": false,
    "actingUser": false
  },
  "MoveTopic": {
    "method": "POST",
    "path": "/topics/{topicId}/moves.json",
    "parameters": [
      {
        "name": "topicId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": true,
    "safe": false,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "GetTopicPublication": {
    "method": "GET",
    "path": "/topics/{topicId}/publication.json",
    "parameters": [
      {
        "name": "topicId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "RestoreTopic": {
    "method": "PUT",
    "path": "/topics/{topicId}/status/active.json",
    "parameters": [
      {
        "name": "topicId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "MarkTopicHam": {
    "method": "PUT",
    "path": "/topics/{topicId}/status/ham.json",
    "parameters": [
      {
        "name": "topicId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "TrashTopic": {
    "method": "PUT",
    "path": "/topics/{topicId}/status/trashed.json",
    "parameters": [
      {
        "name": "topicId",
        "in": "path",
        "required": true,
        "type": "integer"
      },
      {
        "name": "confirm_destroy",
        "in": "query",
        "required": false,
        "type": "string"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 2,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  },
  "MoveWorkflowStaging": {
    "method": "PATCH",
    "path": "/topics/{topicId}/workflows/{workflowId}/stagings",
    "parameters": [
      {
        "name": "topicId",
        "in": "path",
        "required": true,
        "type": "integer"
      },
      {
        "name": "workflowId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": true,
    "safe": false,
    "actingSender": false,
    "actingUser": false
  },
  "CreateWorkflowStaging": {
    "method": "POST",
    "path": "/topics/{topicId}/workflows/{workflowId}/stagings",
    "parameters": [
      {
        "name": "topicId",
        "in": "path",
        "required": true,
        "type": "integer"
      },
      {
        "name": "workflowId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": false,
    "safe": false,
    "actingSender": false,
    "actingUser": false
  },
  "GetWorkflow": {
    "method": "GET",
    "path": "/workflows/{workflowId}",
    "parameters": [
      {
        "name": "workflowId",
        "in": "path",
        "required": true,
        "type": "integer"
      }
    ],
    "bodyRequired": false,
    "safe": true,
    "retry": {
      "maxAttempts": 3,
      "baseDelayMs": 1000,
      "backoff": "exponential",
      "retryOn": [
        429,
        503
      ]
    },
    "actingSender": false,
    "actingUser": false
  }
} as const;
export type OperationName = keyof typeof operationMetadata;

export class GeneratedOperations {
  constructor(protected transport: Transport) {}
  /** Delete an extenzion. The id is the extenzion's contact id, the one its app_url
   * carries. Answers 204; forbidden when the caller cannot edit the extenzion. */
  deleteExtenzion(input: OperationInput<"DeleteExtenzion">, options?: RequestOptions): Promise<OperationResponse<"DeleteExtenzion">> {
    return this.transport.execute("DeleteExtenzion", input, options);
  }

  /** Get the options the advanced search refine form offers.
   *
   * Advanced search: message matches grouped by topic as the search page shows them —
   * the topic, its posting id, and the matching entries as summaries (no bodies; read a
   * message with GetMessage). Refinements are the same query parameters the page uses.
   * The next page, if any, is a Link header. */
  advancedSearch(input: OperationInput<"AdvancedSearch"> = {}, options?: RequestOptions): Promise<OperationResponse<"AdvancedSearch">> {
    return this.transport.execute("AdvancedSearch", input, options);
  }

  /** The advanced search refine form's options: boxes, date ranges, labels and attachment kinds. */
  getAdvancedSearchFilters(input: OperationInput<"GetAdvancedSearchFilters"> = {}, options?: RequestOptions): Promise<OperationResponse<"GetAdvancedSearchFilters">> {
    return this.transport.execute("GetAdvancedSearchFilters", input, options);
  }

  /** List all boxes */
  listBoxes(input: OperationInput<"ListBoxes"> = {}, options?: RequestOptions): Promise<OperationResponse<"ListBoxes">> {
    return this.transport.execute("ListBoxes", input, options);
  }

  /** Get a specific box */
  getBox(input: OperationInput<"GetBox">, options?: RequestOptions): Promise<OperationResponse<"GetBox">> {
    return this.transport.execute("GetBox", input, options);
  }

  /** Designate a contact to a box, so everything they send lands there */
  createBoxDesignation(input: OperationInput<"CreateBoxDesignation">, options?: RequestOptions): Promise<OperationResponse<"CreateBoxDesignation">> {
    return this.transport.execute("CreateBoxDesignation", input, options);
  }

  /** Remove a designation from a box. The id is the designation's, not the contact's. */
  deleteBoxDesignation(input: OperationInput<"DeleteBoxDesignation">, options?: RequestOptions): Promise<OperationResponse<"DeleteBoxDesignation">> {
    return this.transport.execute("DeleteBoxDesignation", input, options);
  }

  /** List the Set Aside groups in a box */
  listBoxGroups(input: OperationInput<"ListBoxGroups">, options?: RequestOptions): Promise<OperationResponse<"ListBoxGroups">> {
    return this.transport.execute("ListBoxGroups", input, options);
  }

  /** Create a Set Aside group out of a selection of postings.
   *
   * This endpoint does not split a comma-joined posting_ids string — send an array. */
  createBoxGroup(input: OperationInput<"CreateBoxGroup">, options?: RequestOptions): Promise<OperationResponse<"CreateBoxGroup">> {
    return this.transport.execute("CreateBoxGroup", input, options);
  }

  /** Break up a Set Aside group, moving its postings back to Previously Seen */
  deleteBoxGroup(input: OperationInput<"DeleteBoxGroup">, options?: RequestOptions): Promise<OperationResponse<"DeleteBoxGroup">> {
    return this.transport.execute("DeleteBoxGroup", input, options);
  }

  /** Read one Set Aside group with the postings in it.
   *
   * The postings are paged like a folder's: newest observed first, 30 to a page, with the
   * next page in the Link header and the total in X-Total-Count. */
  getBoxGroup(input: OperationInput<"GetBoxGroup">, options?: RequestOptions): Promise<OperationResponse<"GetBoxGroup">> {
    return this.transport.execute("GetBoxGroup", input, options);
  }

  /** Mark everything in a box as seen. The work is queued, so the effect is eventually consistent. */
  markBoxSeen(input: OperationInput<"MarkBoxSeen">, options?: RequestOptions): Promise<OperationResponse<"MarkBoxSeen">> {
    return this.transport.execute("MarkBoxSeen", input, options);
  }

  /** Read what changed among a box's postings since a point in time.
   *
   * This is the incremental sync feed the mail clients follow rather than re-reading a
   * box. `since` is an ISO 8601 timestamp with milliseconds and is exclusive, and `v` is
   * the client's contract version — the server answers 409 when the caller is too far
   * behind for an increment to carry the difference, which means read the box in full
   * instead. A box's own `posting_changes_url` carries the `since` and `v` to start from,
   * and the `Link` header names the next page while one remains and the next `since`
   * cursor on the last page. */
  getBoxPostingChanges(input: OperationInput<"GetBoxPostingChanges">, options?: RequestOptions): Promise<OperationResponse<"GetBoxPostingChanges">> {
    return this.transport.execute("GetBoxPostingChanges", input, options);
  }

  /** Get the Bubble Up box */
  getBubblebox(input: OperationInput<"GetBubblebox"> = {}, options?: RequestOptions): Promise<OperationResponse<"GetBubblebox">> {
    return this.transport.execute("GetBubblebox", input, options);
  }

  /** Send one reply to every entry. Answers what was sent, not the replies themselves:
   * delivery is queued, and delayed while undo is still possible. */
  createBulkReply(input: OperationInput<"CreateBulkReply">, options?: RequestOptions): Promise<OperationResponse<"CreateBulkReply">> {
    return this.transport.execute("CreateBulkReply", input, options);
  }

  /** Work out which entries a bulk reply would answer. HEY replies to the last replyable
   * entry of each thread, skipping threads with no reply address, so the postings you hold
   * are not the entries you send to — this resolves them. */
  newBulkReply(input: OperationInput<"NewBulkReply">, options?: RequestOptions): Promise<OperationResponse<"NewBulkReply">> {
    return this.transport.execute("NewBulkReply", input, options);
  }

  /** List the days from a date onwards. The server picks how many, so this is a window
   * rather than a page: read the next one by asking from the last day's date. */
  listCalendarDays(input: OperationInput<"ListCalendarDays"> = {}, options?: RequestOptions): Promise<OperationResponse<"ListCalendarDays">> {
    return this.transport.execute("ListCalendarDays", input, options);
  }

  /** Get one day */
  getCalendarDay(input: OperationInput<"GetCalendarDay">, options?: RequestOptions): Promise<OperationResponse<"GetCalendarDay">> {
    return this.transport.execute("GetCalendarDay", input, options);
  }

  /** Uncomplete a habit for a day */
  uncompleteHabit(input: OperationInput<"UncompleteHabit">, options?: RequestOptions): Promise<OperationResponse<"UncompleteHabit">> {
    return this.transport.execute("UncompleteHabit", input, options);
  }

  /** Complete a habit for a day */
  completeHabit(input: OperationInput<"CompleteHabit">, options?: RequestOptions): Promise<OperationResponse<"CompleteHabit">> {
    return this.transport.execute("CompleteHabit", input, options);
  }

  /** Get journal entry for a day */
  getJournalEntry(input: OperationInput<"GetJournalEntry">, options?: RequestOptions): Promise<OperationResponse<"GetJournalEntry">> {
    return this.transport.execute("GetJournalEntry", input, options);
  }

  /** Update the journal entry for a day: writes (or creates) it and answers the entry as a
   * recording, or 204 when empty content removes it. */
  updateJournalEntry(input: OperationInput<"UpdateJournalEntry">, options?: RequestOptions): Promise<OperationResponse<"UpdateJournalEntry">> {
    return this.transport.execute("UpdateJournalEntry", input, options);
  }

  /** Delete a calendar event, cancelling it for every attendee. Answers 204. */
  deleteCalendarEvent(input: OperationInput<"DeleteCalendarEvent">, options?: RequestOptions): Promise<OperationResponse<"DeleteCalendarEvent">> {
    return this.transport.execute("DeleteCalendarEvent", input, options);
  }

  /** Delete one day of a repeating event, or that day and every one after it.
   * Answers 204. A single day becomes an exception in the series' schedule; with
   * apply_to_future the series is truncated at the day before, or destroyed if this
   * was its first day. */
  deleteCalendarEventOccurrence(input: OperationInput<"DeleteCalendarEventOccurrence">, options?: RequestOptions): Promise<OperationResponse<"DeleteCalendarEventOccurrence">> {
    return this.transport.execute("DeleteCalendarEventOccurrence", input, options);
  }

  /** Start a new habit. Answers the created habit as a recording. */
  createHabit(input: OperationInput<"CreateHabit">, options?: RequestOptions): Promise<OperationResponse<"CreateHabit">> {
    return this.transport.execute("CreateHabit", input, options);
  }

  /** Delete a habit. habitId is the recording's id. */
  deleteHabit(input: OperationInput<"DeleteHabit">, options?: RequestOptions): Promise<OperationResponse<"DeleteHabit">> {
    return this.transport.execute("DeleteHabit", input, options);
  }

  /** Edit a habit. habitId is the recording's id. */
  updateHabit(input: OperationInput<"UpdateHabit">, options?: RequestOptions): Promise<OperationResponse<"UpdateHabit">> {
    return this.transport.execute("UpdateHabit", input, options);
  }

  /** Resume a paused habit */
  resumeHabit(input: OperationInput<"ResumeHabit">, options?: RequestOptions): Promise<OperationResponse<"ResumeHabit">> {
    return this.transport.execute("ResumeHabit", input, options);
  }

  /** Pause a habit, so it stops appearing on the calendar */
  stopHabit(input: OperationInput<"StopHabit">, options?: RequestOptions): Promise<OperationResponse<"StopHabit">> {
    return this.transport.execute("StopHabit", input, options);
  }

  /** Set which day the identity's calendar weeks start on. Answers the stored
   * preference. The write reaches every HEY client — web, mobile and this SDK
   * read the same identity preference. */
  updateFirstWeekDay(input: OperationInput<"UpdateFirstWeekDay">, options?: RequestOptions): Promise<OperationResponse<"UpdateFirstWeekDay">> {
    return this.transport.execute("UpdateFirstWeekDay", input, options);
  }

  /** List journal entries newest first. The next page, if any, is a Link header.
   * Pass q to search journal entry content. */
  listJournalEntries(input: OperationInput<"ListJournalEntries"> = {}, options?: RequestOptions): Promise<OperationResponse<"ListJournalEntries">> {
    return this.transport.execute("ListJournalEntries", input, options);
  }

  /** Get the ongoing time track (404 = no active track; see ADR-004) */
  getOngoingTimeTrack(input: OperationInput<"GetOngoingTimeTrack"> = {}, options?: RequestOptions): Promise<OperationResponse<"GetOngoingTimeTrack">> {
    return this.transport.execute("GetOngoingTimeTrack", input, options);
  }

  /** Start a new time track. Takes no body: haystack's
   * Calendar::OngoingTimeTracksController#create ignores request parameters and
   * starts a track with defaults; use UpdateTimeTrack to set notes and category_title,
   * which also stops the track. */
  startTimeTrack(input: OperationInput<"StartTimeTrack"> = {}, options?: RequestOptions): Promise<OperationResponse<"StartTimeTrack">> {
    return this.transport.execute("StartTimeTrack", input, options);
  }

  /** List tracked time — completed tracks only, newest-ended first.
   *
   * A running track is not here; read that with GetOngoingTimeTrack. The next page, if
   * any, is a Link header, and the last page carries none, so a nil Link is the end of
   * the list rather than an error.
   *
   * category_id narrows the list to one category and 404s if the calendar has no
   * category by that id.
   *
   * The calendar's categories come back alongside the tracks, so showing or applying the
   * filter does not need ListTimeTrackCategories as well. */
  listTimeTracks(input: OperationInput<"ListTimeTracks"> = {}, options?: RequestOptions): Promise<OperationResponse<"ListTimeTracks">> {
    return this.transport.execute("ListTimeTracks", input, options);
  }

  /** Record a finished stretch of time.
   *
   * JSON callers send the fields flat; Rails wraps them into calendar_time_track itself. */
  createTimeTrack(input: OperationInput<"CreateTimeTrack">, options?: RequestOptions): Promise<OperationResponse<"CreateTimeTrack">> {
    return this.transport.execute("CreateTimeTrack", input, options);
  }

  /** List the calendar's time track categories, alphabetically */
  listTimeTrackCategories(input: OperationInput<"ListTimeTrackCategories"> = {}, options?: RequestOptions): Promise<OperationResponse<"ListTimeTrackCategories">> {
    return this.transport.execute("ListTimeTrackCategories", input, options);
  }

  /** Delete a time track. The id is the recording's. */
  deleteTimeTrack(input: OperationInput<"DeleteTimeTrack">, options?: RequestOptions): Promise<OperationResponse<"DeleteTimeTrack">> {
    return this.transport.execute("DeleteTimeTrack", input, options);
  }

  /** Update a time track (stop by setting ends_at to current time).
   *
   * Every update completes the track, whether or not ends_at is sent, so this cannot
   * be used to adjust a running track: it stops it.
   *
   * Only the fields sent are written, so a partial update leaves the rest of the track
   * alone. A starts_at or ends_at the server cannot parse is a 400, not a 422. */
  updateTimeTrack(input: OperationInput<"UpdateTimeTrack">, options?: RequestOptions): Promise<OperationResponse<"UpdateTimeTrack">> {
    return this.transport.execute("UpdateTimeTrack", input, options);
  }

  /** Create a calendar todo */
  createCalendarTodo(input: OperationInput<"CreateCalendarTodo">, options?: RequestOptions): Promise<OperationResponse<"CreateCalendarTodo">> {
    return this.transport.execute("CreateCalendarTodo", input, options);
  }

  /** Delete a calendar todo */
  deleteCalendarTodo(input: OperationInput<"DeleteCalendarTodo">, options?: RequestOptions): Promise<OperationResponse<"DeleteCalendarTodo">> {
    return this.transport.execute("DeleteCalendarTodo", input, options);
  }

  /** Edit a calendar todo. todoId is the recording's id, and every field of the payload
   * is optional: haystack's `wrap_parameters` accepts title, focused and starts_at, and
   * changes only what is sent. */
  updateCalendarTodo(input: OperationInput<"UpdateCalendarTodo">, options?: RequestOptions): Promise<OperationResponse<"UpdateCalendarTodo">> {
    return this.transport.execute("UpdateCalendarTodo", input, options);
  }

  /** Uncomplete a calendar todo */
  uncompleteCalendarTodo(input: OperationInput<"UncompleteCalendarTodo">, options?: RequestOptions): Promise<OperationResponse<"UncompleteCalendarTodo">> {
    return this.transport.execute("UncompleteCalendarTodo", input, options);
  }

  /** Complete a calendar todo */
  completeCalendarTodo(input: OperationInput<"CompleteCalendarTodo">, options?: RequestOptions): Promise<OperationResponse<"CompleteCalendarTodo">> {
    return this.transport.execute("CompleteCalendarTodo", input, options);
  }

  /** List the weeks around a date — nine of them, centered on it. */
  listCalendarWeeks(input: OperationInput<"ListCalendarWeeks"> = {}, options?: RequestOptions): Promise<OperationResponse<"ListCalendarWeeks">> {
    return this.transport.execute("ListCalendarWeeks", input, options);
  }

  /** Get one week */
  getCalendarWeek(input: OperationInput<"GetCalendarWeek">, options?: RequestOptions): Promise<OperationResponse<"GetCalendarWeek">> {
    return this.transport.execute("GetCalendarWeek", input, options);
  }

  /** Get one year as the grid it is drawn as */
  getCalendarYear(input: OperationInput<"GetCalendarYear">, options?: RequestOptions): Promise<OperationResponse<"GetCalendarYear">> {
    return this.transport.execute("GetCalendarYear", input, options);
  }

  /** List calendars */
  listCalendars(input: OperationInput<"ListCalendars"> = {}, options?: RequestOptions): Promise<OperationResponse<"ListCalendars">> {
    return this.transport.execute("ListCalendars", input, options);
  }

  /** Get recordings for a calendar */
  getCalendarRecordings(input: OperationInput<"GetCalendarRecordings">, options?: RequestOptions): Promise<OperationResponse<"GetCalendarRecordings">> {
    return this.transport.execute("GetCalendarRecordings", input, options);
  }

  /** Switch a calendar in or out of the reader's selection, and answer the selection it
   * left behind. The selection is what every period read is scoped to, so a toggle is how
   * a client changes which calendars a day, week or year is drawn from. */
  toggleCalendar(input: OperationInput<"ToggleCalendar">, options?: RequestOptions): Promise<OperationResponse<"ToggleCalendar">> {
    return this.transport.execute("ToggleCalendar", input, options);
  }

  /** Get the Screener — the pending count, and the senders waiting when asked for them */
  getClearances(input: OperationInput<"GetClearances"> = {}, options?: RequestOptions): Promise<OperationResponse<"GetClearances">> {
    return this.transport.execute("GetClearances", input, options);
  }

  /** Screen several senders out at once. ids is a comma-separated list. */
  bulkUpdateClearances(input: OperationInput<"BulkUpdateClearances">, options?: RequestOptions): Promise<OperationResponse<"BulkUpdateClearances">> {
    return this.transport.execute("BulkUpdateClearances", input, options);
  }

  /** Clear the Screener — every pending sender is punted and reexamined on their next email */
  puntClearances(input: OperationInput<"PuntClearances"> = {}, options?: RequestOptions): Promise<OperationResponse<"PuntClearances">> {
    return this.transport.execute("PuntClearances", input, options);
  }

  /** Screen a sender in or out of the Screener
   *
   * designation_box_id files everything they send into that box instead of the Imbox.
   * spam marks what is already waiting as spam and trains the filter on it. */
  updateClearance(input: OperationInput<"UpdateClearance">, options?: RequestOptions): Promise<OperationResponse<"UpdateClearance">> {
    return this.transport.execute("UpdateClearance", input, options);
  }

  /** List clips, newest first */
  listClips(input: OperationInput<"ListClips"> = {}, options?: RequestOptions): Promise<OperationResponse<"ListClips">> {
    return this.transport.execute("ListClips", input, options);
  }

  /** List collections */
  listCollections(input: OperationInput<"ListCollections"> = {}, options?: RequestOptions): Promise<OperationResponse<"ListCollections">> {
    return this.transport.execute("ListCollections", input, options);
  }

  /** Get a collection and one page of its active, accessible threads */
  getCollection(input: OperationInput<"GetCollection">, options?: RequestOptions): Promise<OperationResponse<"GetCollection">> {
    return this.transport.execute("GetCollection", input, options);
  }

  /** Rename a collection or change its summary */
  updateCollection(input: OperationInput<"UpdateCollection">, options?: RequestOptions): Promise<OperationResponse<"UpdateCollection">> {
    return this.transport.execute("UpdateCollection", input, options);
  }

  /** List contacts */
  listContacts(input: OperationInput<"ListContacts"> = {}, options?: RequestOptions): Promise<OperationResponse<"ListContacts">> {
    return this.transport.execute("ListContacts", input, options);
  }

  /** Add a contact. Answers the contact that was created. */
  createContact(input: OperationInput<"CreateContact">, options?: RequestOptions): Promise<OperationResponse<"CreateContact">> {
    return this.transport.execute("CreateContact", input, options);
  }

  /** Hide a contact. Nothing is deleted — RevealContact brings them back. */
  hideContact(input: OperationInput<"HideContact">, options?: RequestOptions): Promise<OperationResponse<"HideContact">> {
    return this.transport.execute("HideContact", input, options);
  }

  /** Get a contact, with a page of the threads they are on */
  getContact(input: OperationInput<"GetContact">, options?: RequestOptions): Promise<OperationResponse<"GetContact">> {
    return this.transport.execute("GetContact", input, options);
  }

  /** Edit a contact. HEY rewrites the whole contact, so send every field: a name,
   * address or alias left out is cleared. Answers the contact, which is not always
   * the one addressed — promoting an alias makes the alias primary. */
  updateContact(input: OperationInput<"UpdateContact">, options?: RequestOptions): Promise<OperationResponse<"UpdateContact">> {
    return this.transport.execute("UpdateContact", input, options);
  }

  /** Stop bundling a contact's mail */
  unbundleContact(input: OperationInput<"UnbundleContact">, options?: RequestOptions): Promise<OperationResponse<"UnbundleContact">> {
    return this.transport.execute("UnbundleContact", input, options);
  }

  /** Bundle a contact so their mail arrives grouped */
  bundleContact(input: OperationInput<"BundleContact">, options?: RequestOptions): Promise<OperationResponse<"BundleContact">> {
    return this.transport.execute("BundleContact", input, options);
  }

  /** Screen a contact in or out. Status is "approved" or "denied". */
  updateContactClearance(input: OperationInput<"UpdateContactClearance">, options?: RequestOptions): Promise<OperationResponse<"UpdateContactClearance">> {
    return this.transport.execute("UpdateContactClearance", input, options);
  }

  /** Clear the private note on a contact */
  deleteContactNote(input: OperationInput<"DeleteContactNote">, options?: RequestOptions): Promise<OperationResponse<"DeleteContactNote">> {
    return this.transport.execute("DeleteContactNote", input, options);
  }

  /** Read the private note kept on a contact */
  getContactNote(input: OperationInput<"GetContactNote">, options?: RequestOptions): Promise<OperationResponse<"GetContactNote">> {
    return this.transport.execute("GetContactNote", input, options);
  }

  /** Write the private note on a contact, replacing whatever was there */
  updateContactNote(input: OperationInput<"UpdateContactNote">, options?: RequestOptions): Promise<OperationResponse<"UpdateContactNote">> {
    return this.transport.execute("UpdateContactNote", input, options);
  }

  /** Put a hidden contact back in the contact list */
  revealContact(input: OperationInput<"RevealContact">, options?: RequestOptions): Promise<OperationResponse<"RevealContact">> {
    return this.transport.execute("RevealContact", input, options);
  }

  /** List draft messages */
  listDrafts(input: OperationInput<"ListDrafts"> = {}, options?: RequestOptions): Promise<OperationResponse<"ListDrafts">> {
    return this.transport.execute("ListDrafts", input, options);
  }

  /** Trash a draft (Entries::DraftsController#destroy). The id is the draft's entry id,
   * as ListDrafts reports it. */
  deleteDraft(input: OperationInput<"DeleteDraft">, options?: RequestOptions): Promise<OperationResponse<"DeleteDraft">> {
    return this.transport.execute("DeleteDraft", input, options);
  }

  /** Get a prefilled forward of an entry: subject, quoted body and blank recipients.
   * Send it with CreateMessage once the recipients are filled in. */
  newEntryForward(input: OperationInput<"NewEntryForward">, options?: RequestOptions): Promise<OperationResponse<"NewEntryForward">> {
    return this.transport.execute("NewEntryForward", input, options);
  }

  /** Reply to an entry */
  createReply(input: OperationInput<"CreateReply">, options?: RequestOptions): Promise<OperationResponse<"CreateReply">> {
    return this.transport.execute("CreateReply", input, options);
  }

  /** Get a prefilled reply to an entry: the quoted body and, in addressed, the
   * participating contacts a reply goes to as HEY computes them — the sender moved onto
   * the To line and the acting user's own addresses, aliases and catch-alls excluded. */
  newEntryReply(input: OperationInput<"NewEntryReply">, options?: RequestOptions): Promise<OperationResponse<"NewEntryReply">> {
    return this.transport.execute("NewEntryReply", input, options);
  }

  /** Mark an entry as spam. Denies the sender when every thread from them is already spam. */
  markEntrySpam(input: OperationInput<"MarkEntrySpam">, options?: RequestOptions): Promise<OperationResponse<"MarkEntrySpam">> {
    return this.transport.execute("MarkEntrySpam", input, options);
  }

  /** Get the Feed */
  getFeedbox(input: OperationInput<"GetFeedbox"> = {}, options?: RequestOptions): Promise<OperationResponse<"GetFeedbox">> {
    return this.transport.execute("GetFeedbox", input, options);
  }

  /** Get a folder (label) and the postings filed in it */
  getFolder(input: OperationInput<"GetFolder">, options?: RequestOptions): Promise<OperationResponse<"GetFolder">> {
    return this.transport.execute("GetFolder", input, options);
  }

  /** Get the current identity (authenticated user profile) */
  getIdentity(input: OperationInput<"GetIdentity"> = {}, options?: RequestOptions): Promise<OperationResponse<"GetIdentity">> {
    return this.transport.execute("GetIdentity", input, options);
  }

  /** Set whether HEY renders times on a 12-hour or a 24-hour clock. Answers the
   * stored preference. The parameter is the web toggle's, said honestly: true
   * for the 24-hour clock, false for the 12-hour one. */
  updateTimeFormat(input: OperationInput<"UpdateTimeFormat">, options?: RequestOptions): Promise<OperationResponse<"UpdateTimeFormat">> {
    return this.transport.execute("UpdateTimeFormat", input, options);
  }

  /** Get the Imbox */
  getImbox(input: OperationInput<"GetImbox"> = {}, options?: RequestOptions): Promise<OperationResponse<"GetImbox">> {
    return this.transport.execute("GetImbox", input, options);
  }

  /** Get the Imbox's Previously Seen postings */
  getImboxSeen(input: OperationInput<"GetImboxSeen"> = {}, options?: RequestOptions): Promise<OperationResponse<"GetImboxSeen">> {
    return this.transport.execute("GetImboxSeen", input, options);
  }

  /** Create a new message (start a new topic).
   * The acting sender ID must be included; the Go SDK resolves this automatically.
   * Every message is created drafted on HEY's side; without entry.status the server
   * delivers it, while entry.status "drafted" leaves it as a draft and answers
   * 204 with a Location header naming /messages/{entry_id}. */
  createMessage(input: OperationInput<"CreateMessage">, options?: RequestOptions): Promise<OperationResponse<"CreateMessage">> {
    return this.transport.execute("CreateMessage", input, options);
  }

  /** Get a message */
  getMessage(input: OperationInput<"GetMessage">, options?: RequestOptions): Promise<OperationResponse<"GetMessage">> {
    return this.transport.execute("GetMessage", input, options);
  }

  /** Revise a message entry (MessagesController#update). With entry.status "drafted" the
   * entry is saved as a draft (204 + Location, like CreateMessage); without it a draft is
   * delivered through the undo-delay window. A trashed draft is silently restored first.
   * The revision is not a patch: subject, content and any scheduled delivery are rewritten
   * from this request (an omitted scheduled delivery clears one), while recipients are
   * replaced only when entry.addressed is present.
   *
   * Not naturally idempotent despite the PUT: without the drafted status this request
   * *delivers*, so a transparent retry after an ambiguous first attempt could send the
   * message again. The client must not retry it. */
  updateMessage(input: OperationInput<"UpdateMessage">, options?: RequestOptions): Promise<OperationResponse<"UpdateMessage">> {
    return this.transport.execute("UpdateMessage", input, options);
  }

  /** A draft's editable state: content, recipients and scheduled delivery as the
   * composer would load them (GET /messages/{id}/edit). */
  getMessageEdit(input: OperationInput<"GetMessageEdit">, options?: RequestOptions): Promise<OperationResponse<"GetMessageEdit">> {
    return this.transport.execute("GetMessageEdit", input, options);
  }

  /** The senders already screened in or out */
  getMyClearances(input: OperationInput<"GetMyClearances"> = {}, options?: RequestOptions): Promise<OperationResponse<"GetMyClearances">> {
    return this.transport.execute("GetMyClearances", input, options);
  }

  /** Rescreen a sender who was already screened in or out */
  updateMyClearance(input: OperationInput<"UpdateMyClearance">, options?: RequestOptions): Promise<OperationResponse<"UpdateMyClearance">> {
    return this.transport.execute("UpdateMyClearance", input, options);
  }

  /** Get navigation items */
  getNavigation(input: OperationInput<"GetNavigation"> = {}, options?: RequestOptions): Promise<OperationResponse<"GetNavigation">> {
    return this.transport.execute("GetNavigation", input, options);
  }

  /** Get the Paper Trail */
  getTrailbox(input: OperationInput<"GetTrailbox"> = {}, options?: RequestOptions): Promise<OperationResponse<"GetTrailbox">> {
    return this.transport.execute("GetTrailbox", input, options);
  }

  /** Remove a selection of postings from their Set Aside group */
  removePostingsFromBoxGroup(input: OperationInput<"RemovePostingsFromBoxGroup">, options?: RequestOptions): Promise<OperationResponse<"RemovePostingsFromBoxGroup">> {
    return this.transport.execute("RemovePostingsFromBoxGroup", input, options);
  }

  /** Add a selection of postings to a Set Aside group */
  addPostingsToBoxGroup(input: OperationInput<"AddPostingsToBoxGroup">, options?: RequestOptions): Promise<OperationResponse<"AddPostingsToBoxGroup">> {
    return this.transport.execute("AddPostingsToBoxGroup", input, options);
  }

  /** Cancel a scheduled bubble up for a selection of postings */
  cancelPostingsBubbleUp(input: OperationInput<"CancelPostingsBubbleUp">, options?: RequestOptions): Promise<OperationResponse<"CancelPostingsBubbleUp">> {
    return this.transport.execute("CancelPostingsBubbleUp", input, options);
  }

  /** Schedule a selection of postings to bubble up.
   *
   * HEY's scheduler takes a `slot` — today, tomorrow, weekend, next_week, surprise_me
   * or custom — and a custom slot also carries the `date` (YYYY-MM-DD) to bubble up on,
   * at HEY's morning hour. The today slot lands at HEY's evening hour of the current
   * day instead, and both hours are UTC over JSON. An unknown slot, or a custom slot
   * without a date, is a server error rather than a validation response, so callers
   * check both first. Responds 201 Created. */
  schedulePostingsBubbleUp(input: OperationInput<"SchedulePostingsBubbleUp">, options?: RequestOptions): Promise<OperationResponse<"SchedulePostingsBubbleUp">> {
    return this.transport.execute("SchedulePostingsBubbleUp", input, options);
  }

  /** Bubble a selection of postings up right now */
  bubbleUpPostingsNow(input: OperationInput<"BubbleUpPostingsNow">, options?: RequestOptions): Promise<OperationResponse<"BubbleUpPostingsNow">> {
    return this.transport.execute("BubbleUpPostingsNow", input, options);
  }

  /** Remove a selection of postings from a folder, or from every folder when folder_id is omitted */
  unfilePostings(input: OperationInput<"UnfilePostings">, options?: RequestOptions): Promise<OperationResponse<"UnfilePostings">> {
    return this.transport.execute("UnfilePostings", input, options);
  }

  /** File a selection of postings into an existing folder (label) */
  filePostings(input: OperationInput<"FilePostings">, options?: RequestOptions): Promise<OperationResponse<"FilePostings">> {
    return this.transport.execute("FilePostings", input, options);
  }

  /** Create a folder (label) and file a selection of postings into it */
  createFolderForPostings(input: OperationInput<"CreateFolderForPostings">, options?: RequestOptions): Promise<OperationResponse<"CreateFolderForPostings">> {
    return this.transport.execute("CreateFolderForPostings", input, options);
  }

  /** Move postings to a box (bulk).
   * Mirrors HEY's Postings::MovesController: `posting_ids` plus the target `box_id`
   * (an ID from ListBoxes; the box `kind` field identifies imbox, feedbox, asidebox,
   * laterbox, trailbox). Responds 204 No Content. */
  movePostings(input: OperationInput<"MovePostings">, options?: RequestOptions): Promise<OperationResponse<"MovePostings">> {
    return this.transport.execute("MovePostings", input, options);
  }

  /** Unmute postings (bulk).
   * Mirrors HEY's Postings::MutingsController#destroy. `posting_ids` is sent as a
   * comma-separated query string because DELETE carries no body. Responds 201 Created. */
  unmutePostings(input: OperationInput<"UnmutePostings">, options?: RequestOptions): Promise<OperationResponse<"UnmutePostings">> {
    return this.transport.execute("UnmutePostings", input, options);
  }

  /** Mute postings (bulk) — stop notifications for their threads.
   * Mirrors HEY's Postings::MutingsController#create. Responds 201 Created. */
  mutePostings(input: OperationInput<"MutePostings">, options?: RequestOptions): Promise<OperationResponse<"MutePostings">> {
    return this.transport.execute("MutePostings", input, options);
  }

  /** Mark postings as seen */
  markPostingsSeen(input: OperationInput<"MarkPostingsSeen">, options?: RequestOptions): Promise<OperationResponse<"MarkPostingsSeen">> {
    return this.transport.execute("MarkPostingsSeen", input, options);
  }

  /** Mark a selection of postings as spam.
   *
   * Over ten postings the server hands the work to a background job, so the effect is
   * eventually consistent. */
  markPostingsSpam(input: OperationInput<"MarkPostingsSpam">, options?: RequestOptions): Promise<OperationResponse<"MarkPostingsSpam">> {
    return this.transport.execute("MarkPostingsSpam", input, options);
  }

  /** Move postings to the trash (bulk).
   * Mirrors HEY's Postings::TrashController. For JSON requests the server treats
   * the removal decision as made (shared topics: your access is removed).
   * Responds 204 No Content. */
  trashPostings(input: OperationInput<"TrashPostings">, options?: RequestOptions): Promise<OperationResponse<"TrashPostings">> {
    return this.transport.execute("TrashPostings", input, options);
  }

  /** Mark postings as unseen */
  markPostingsUnseen(input: OperationInput<"MarkPostingsUnseen">, options?: RequestOptions): Promise<OperationResponse<"MarkPostingsUnseen">> {
    return this.transport.execute("MarkPostingsUnseen", input, options);
  }

  /** List the unseen postings inside a bundle posting.
   *
   * A bundle posting groups one contact's unseen mail; this is its contents — the member
   * postings, newest first, paged by cursor like a box. The posting must be a bundle. */
  getBundleUnseenPostings(input: OperationInput<"GetBundleUnseenPostings">, options?: RequestOptions): Promise<OperationResponse<"GetBundleUnseenPostings">> {
    return this.transport.execute("GetBundleUnseenPostings", input, options);
  }

  /** Create an Active Storage direct upload for an outgoing attachment.
   * The returned URL is self-authenticating and accepts the raw file bytes via PUT. */
  createDirectUpload(input: OperationInput<"CreateDirectUpload">, options?: RequestOptions): Promise<OperationResponse<"CreateDirectUpload">> {
    return this.transport.execute("CreateDirectUpload", input, options);
  }

  /** Get the Reply Later box */
  getLaterbox(input: OperationInput<"GetLaterbox"> = {}, options?: RequestOptions): Promise<OperationResponse<"GetLaterbox">> {
    return this.transport.execute("GetLaterbox", input, options);
  }

  /** Get the Set Aside box */
  getAsidebox(input: OperationInput<"GetAsidebox"> = {}, options?: RequestOptions): Promise<OperationResponse<"GetAsidebox">> {
    return this.transport.execute("GetAsidebox", input, options);
  }

  /** List snippets, alphabetically */
  listSnippets(input: OperationInput<"ListSnippets"> = {}, options?: RequestOptions): Promise<OperationResponse<"ListSnippets">> {
    return this.transport.execute("ListSnippets", input, options);
  }

  /** List stickies, newest position first */
  listStickies(input: OperationInput<"ListStickies"> = {}, options?: RequestOptions): Promise<OperationResponse<"ListStickies">> {
    return this.transport.execute("ListStickies", input, options);
  }

  /** Write a new sticky */
  createSticky(input: OperationInput<"CreateSticky">, options?: RequestOptions): Promise<OperationResponse<"CreateSticky">> {
    return this.transport.execute("CreateSticky", input, options);
  }

  /** Reposition a sticky on the board */
  moveSticky(input: OperationInput<"MoveSticky">, options?: RequestOptions): Promise<OperationResponse<"MoveSticky">> {
    return this.transport.execute("MoveSticky", input, options);
  }

  /** Throw a sticky away */
  deleteSticky(input: OperationInput<"DeleteSticky">, options?: RequestOptions): Promise<OperationResponse<"DeleteSticky">> {
    return this.transport.execute("DeleteSticky", input, options);
  }

  /** Edit a sticky */
  updateSticky(input: OperationInput<"UpdateSticky">, options?: RequestOptions): Promise<OperationResponse<"UpdateSticky">> {
    return this.transport.execute("UpdateSticky", input, options);
  }

  /** Get all topics (everything view) */
  getEverythingTopics(input: OperationInput<"GetEverythingTopics"> = {}, options?: RequestOptions): Promise<OperationResponse<"GetEverythingTopics">> {
    return this.transport.execute("GetEverythingTopics", input, options);
  }

  /** Get sent topics */
  getSentTopics(input: OperationInput<"GetSentTopics"> = {}, options?: RequestOptions): Promise<OperationResponse<"GetSentTopics">> {
    return this.transport.execute("GetSentTopics", input, options);
  }

  /** Get spam topics */
  getSpamTopics(input: OperationInput<"GetSpamTopics"> = {}, options?: RequestOptions): Promise<OperationResponse<"GetSpamTopics">> {
    return this.transport.execute("GetSpamTopics", input, options);
  }

  /** Empty the spam box. Runs synchronously, so it can take a while on a large mailbox. */
  emptySpam(input: OperationInput<"EmptySpam"> = {}, options?: RequestOptions): Promise<OperationResponse<"EmptySpam">> {
    return this.transport.execute("EmptySpam", input, options);
  }

  /** Get trash topics */
  getTrashTopics(input: OperationInput<"GetTrashTopics"> = {}, options?: RequestOptions): Promise<OperationResponse<"GetTrashTopics">> {
    return this.transport.execute("GetTrashTopics", input, options);
  }

  /** Empty the trash. Runs synchronously, so it can take a while on a large mailbox. */
  emptyTrash(input: OperationInput<"EmptyTrash"> = {}, options?: RequestOptions): Promise<OperationResponse<"EmptyTrash">> {
    return this.transport.execute("EmptyTrash", input, options);
  }

  /** Get a topic */
  getTopic(input: OperationInput<"GetTopic">, options?: RequestOptions): Promise<OperationResponse<"GetTopic">> {
    return this.transport.execute("GetTopic", input, options);
  }

  /** Get entries for a topic */
  getTopicEntries(input: OperationInput<"GetTopicEntries">, options?: RequestOptions): Promise<OperationResponse<"GetTopicEntries">> {
    return this.transport.execute("GetTopicEntries", input, options);
  }

  /** Move a topic to another box.
   *
   * Answers 204 without moving anything when the acting user has no posting for the topic. */
  moveTopic(input: OperationInput<"MoveTopic">, options?: RequestOptions): Promise<OperationResponse<"MoveTopic">> {
    return this.transport.execute("MoveTopic", input, options);
  }

  /** Whether a thread is shared with a public link, and the link */
  getTopicPublication(input: OperationInput<"GetTopicPublication">, options?: RequestOptions): Promise<OperationResponse<"GetTopicPublication">> {
    return this.transport.execute("GetTopicPublication", input, options);
  }

  /** Restore a topic from the trash or the catch-all */
  restoreTopic(input: OperationInput<"RestoreTopic">, options?: RequestOptions): Promise<OperationResponse<"RestoreTopic">> {
    return this.transport.execute("RestoreTopic", input, options);
  }

  /** Mark a spam topic as ham. Every other spam topic from the same sender is hammed too. */
  markTopicHam(input: OperationInput<"MarkTopicHam">, options?: RequestOptions): Promise<OperationResponse<"MarkTopicHam">> {
    return this.transport.execute("MarkTopicHam", input, options);
  }

  /** Trash a topic.
   *
   * A shared topic redirects to the removal confirmation page unless confirm_destroy is set,
   * so always pass it when trashing something that might be shared. */
  trashTopic(input: OperationInput<"TrashTopic">, options?: RequestOptions): Promise<OperationResponse<"TrashTopic">> {
    return this.transport.execute("TrashTopic", input, options);
  }

  /** Move a staged topic to a workflow stage. */
  moveWorkflowStaging(input: OperationInput<"MoveWorkflowStaging">, options?: RequestOptions): Promise<OperationResponse<"MoveWorkflowStaging">> {
    return this.transport.execute("MoveWorkflowStaging", input, options);
  }

  /** Add a topic to a workflow. HEY places it in the first stage. */
  createWorkflowStaging(input: OperationInput<"CreateWorkflowStaging">, options?: RequestOptions): Promise<OperationResponse<"CreateWorkflowStaging">> {
    return this.transport.execute("CreateWorkflowStaging", input, options);
  }

  /** A workflow with its stages */
  getWorkflow(input: OperationInput<"GetWorkflow">, options?: RequestOptions): Promise<OperationResponse<"GetWorkflow">> {
    return this.transport.execute("GetWorkflow", input, options);
  }
}
