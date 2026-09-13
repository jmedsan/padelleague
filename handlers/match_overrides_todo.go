package handlers

// TODO(B9): detectFieldChange compares the posted date value (YYYY-MM-DD) against
// match.GetString("date") which returns "2026-09-20 00:00:00.000Z" for a PocketBase
// DateField. They never match, so every admin override logs a phantom "Fecha cambiada"
// timeline entry, sends notifications to all four players, and calls ClearMatchReminders
// even when only the score changed.
//
// Fix in detectFieldChange (match.go ~line 439-448): normalize both sides to YYYY-MM-DD
// before comparing by taking the first 10 characters:
//
//   posted  := strings.TrimSpace(strings.Split(newVal, " ")[0])
//   stored  := strings.TrimSpace(strings.Split(oldVal, " ")[0])
//   if posted == stored { return false }
//
// Same normalization applies to the "time" field (HH:MM vs HH:MM:SS.000Z).
// Add a handler test: "admin override score only → single change, no date/time entry".
