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

export function isRecordingCalendarEvent(value: components['schemas']["Recording"]): value is components['schemas']["Recording"] & { "type": "CalendarEvent" | "Calendar::Event" } {
  return value["type"] === "CalendarEvent" || value["type"] === "Calendar::Event";
}

export function isRecordingCalendarTodo(value: components['schemas']["Recording"]): value is components['schemas']["Recording"] & { "type": "CalendarTodo" | "Calendar::Todo" } {
  return value["type"] === "CalendarTodo" || value["type"] === "Calendar::Todo";
}

export function isRecordingCalendarJournalEntry(value: components['schemas']["Recording"]): value is components['schemas']["Recording"] & { "type": "CalendarJournalEntry" | "Calendar::JournalEntry" } {
  return value["type"] === "CalendarJournalEntry" || value["type"] === "Calendar::JournalEntry";
}

export function isRecordingCalendarHabit(value: components['schemas']["Recording"]): value is components['schemas']["Recording"] & { "type": "CalendarHabit" | "Calendar::Habit" } {
  return value["type"] === "CalendarHabit" || value["type"] === "Calendar::Habit";
}

export function isRecordingCalendarTimeTrack(value: components['schemas']["Recording"]): value is components['schemas']["Recording"] & { "type": "CalendarTimeTrack" | "Calendar::TimeTrack" } {
  return value["type"] === "CalendarTimeTrack" || value["type"] === "Calendar::TimeTrack";
}

export function isRecordingCalendarCountdown(value: components['schemas']["Recording"]): value is components['schemas']["Recording"] & { "type": "CalendarCountdown" | "Calendar::Countdown" } {
  return value["type"] === "CalendarCountdown" || value["type"] === "Calendar::Countdown";
}

export function isRecordingCalendarDayBackground(value: components['schemas']["Recording"]): value is components['schemas']["Recording"] & { "type": "CalendarDayBackground" | "Calendar::DayBackground" } {
  return value["type"] === "CalendarDayBackground" || value["type"] === "Calendar::DayBackground";
}
