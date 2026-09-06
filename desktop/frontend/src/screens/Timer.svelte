<script lang="ts">
  import { timer } from "../lib/stores";
  import { mmss } from "../lib/format";
  import {
    Pause,
    Resume,
    Cancel,
    StartBreak,
    SkipBreak,
  } from "../../wailsjs/go/main/TimerService";

  const R = 54;
  const C = 2 * Math.PI * R;

  $: t = $timer;
  $: frac = t.duration > 0 ? Math.max(0, Math.min(1, t.remaining / t.duration)) : 0;
  $: offset = C * (1 - frac);

  const labels: Record<string, string> = {
    running: "Focus",
    paused: "Paused",
    break_prompt: "Time for a break",
    break: "Break",
  };
</script>

<section>
  <div class="ring">
    <svg viewBox="0 0 120 120">
      <circle cx="60" cy="60" r={R} class="track" />
      <circle
        cx="60"
        cy="60"
        r={R}
        class="progress"
        stroke-dasharray={C}
        stroke-dashoffset={offset}
      />
    </svg>
    <div class="countdown">{mmss(t.remaining)}</div>
  </div>

  <h2>{t.task || "Focus"}</h2>
  <p class="muted">{labels[t.phase] ?? t.phase}</p>

  <div class="actions">
    {#if t.phase === "running"}
      <button on:click={() => Pause()}>Pause</button>
      <button class="ghost" on:click={() => Cancel()}>Cancel</button>
    {:else if t.phase === "paused"}
      <button on:click={() => Resume()}>Resume</button>
      <button class="ghost" on:click={() => Cancel()}>Cancel</button>
    {:else if t.phase === "break_prompt"}
      <button on:click={() => StartBreak(5)}>Start break (5m)</button>
      <button class="ghost" on:click={() => SkipBreak()}>Skip</button>
    {:else if t.phase === "break"}
      <button class="ghost" on:click={() => SkipBreak()}>Skip</button>
    {/if}
  </div>
</section>

<style>
  section {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.75rem;
  }
  .ring {
    position: relative;
    width: 220px;
    height: 220px;
  }
  .ring svg {
    width: 100%;
    height: 100%;
    transform: rotate(-90deg);
  }
  .track {
    fill: none;
    stroke: var(--muted);
    stroke-width: 6;
    opacity: 0.3;
  }
  .progress {
    fill: none;
    stroke: var(--accent);
    stroke-width: 6;
    stroke-linecap: round;
    transition: stroke-dashoffset 0.5s linear;
  }
  .countdown {
    position: absolute;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 2.5rem;
  }
  h2 {
    margin: 0;
  }
  .actions {
    display: flex;
    gap: 0.5rem;
    margin-top: 0.5rem;
  }
  .actions button {
    background: var(--accent);
    border: none;
    color: #fff;
    padding: 0.5rem 1rem;
    border-radius: 4px;
    cursor: pointer;
    font-size: 0.95rem;
  }
  .actions button.ghost {
    background: transparent;
    border: 1px solid var(--muted);
    color: var(--fg);
  }
</style>
