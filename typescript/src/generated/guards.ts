// Generated from openapi.json and behavior-model.json. Do not edit.
import type { components } from './schema.js';
export function isPostingTopic(value: components['schemas']["Posting"]): value is components['schemas']["Posting"] & { "kind": "topic" } {
  return value["kind"] === "topic";
}

export function isPostingBundle(value: components['schemas']["Posting"]): value is components['schemas']["Posting"] & { "kind": "bundle" } {
  return value["kind"] === "bundle";
}

export function isPostingEntry(value: components['schemas']["Posting"]): value is components['schemas']["Posting"] & { "kind": "entry" } {
  return value["kind"] === "entry";
}

export function isRecordingCalendarEvent(value: components['schemas']["Recording"]): value is components['schemas']["Recording"] & { "type": "CalendarEvent" } {
  return value["type"] === "CalendarEvent";
}

export function isRecordingCalendarTodo(value: components['schemas']["Recording"]): value is components['schemas']["Recording"] & { "type": "CalendarTodo" } {
  return value["type"] === "CalendarTodo";
}

export function isRecordingCalendarJournalEntry(value: components['schemas']["Recording"]): value is components['schemas']["Recording"] & { "type": "CalendarJournalEntry" } {
  return value["type"] === "CalendarJournalEntry";
}

export function isRecordingCalendarHabit(value: components['schemas']["Recording"]): value is components['schemas']["Recording"] & { "type": "CalendarHabit" } {
  return value["type"] === "CalendarHabit";
}

export function isRecordingCalendarTimeTrack(value: components['schemas']["Recording"]): value is components['schemas']["Recording"] & { "type": "CalendarTimeTrack" } {
  return value["type"] === "CalendarTimeTrack";
}

export function isRecordingCalendarCountdown(value: components['schemas']["Recording"]): value is components['schemas']["Recording"] & { "type": "CalendarCountdown" } {
  return value["type"] === "CalendarCountdown";
}

export function isRecordingCalendarDayBackground(value: components['schemas']["Recording"]): value is components['schemas']["Recording"] & { "type": "CalendarDayBackground" } {
  return value["type"] === "CalendarDayBackground";
}
