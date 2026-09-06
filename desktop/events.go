package main

// Event names emitted to the frontend over the Wails runtime event bus.
const (
	EventTick  = "timer:tick"
	EventPhase = "timer:phase"
	EventError = "timer:error"
	EventToday = "today:changed"
)
