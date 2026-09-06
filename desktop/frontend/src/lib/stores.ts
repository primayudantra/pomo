import { writable } from "svelte/store";
import { GetState } from "../../wailsjs/go/main/TimerService";
import { Today as FetchToday } from "../../wailsjs/go/main/SessionService";
import { Status as DaemonStatusCall } from "../../wailsjs/go/main/DaemonService";
import { EventsOn } from "../../wailsjs/runtime/runtime";

export type Phase = "idle" | "running" | "paused" | "break_prompt" | "break";

export interface State {
  phase: Phase;
  task: string;
  remaining: number;
  duration: number;
  sessionId: number;
}

export interface SessionRow {
  task: string;
  minutes: number;
  status: string;
  startedAt: string;
}

export interface TodayView {
  date: string;
  focusMinutes: number;
  count: number;
  sessions: SessionRow[];
}

export interface DaemonStatus {
  running: boolean;
  pid: number;
}

export const timer = writable<State>({
  phase: "idle",
  task: "",
  remaining: 0,
  duration: 0,
  sessionId: 0,
});

export const today = writable<TodayView>({
  date: "",
  focusMinutes: 0,
  count: 0,
  sessions: [],
});

export const daemon = writable<DaemonStatus>({ running: false, pid: 0 });

let started = false;

// initStores fetches the initial backend state and subscribes to live events.
// Safe to call once from App.svelte's onMount.
export async function initStores(): Promise<void> {
  if (started) return;
  started = true;

  try {
    const [s, t, d] = await Promise.all([GetState(), FetchToday(), DaemonStatusCall()]);
    timer.set(s as State);
    today.set(t as TodayView);
    daemon.set(d as DaemonStatus);
  } catch (err) {
    console.error("initStores: initial fetch failed", err);
  }

  EventsOn("timer:tick", (s: State) => timer.set(s));
  EventsOn("timer:phase", (s: State) => timer.set(s));
  EventsOn("today:changed", () => {
    FetchToday().then((t) => today.set(t as TodayView));
  });
}
