<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { today, daemon } from "../lib/stores";
  import { SetTracking, Status } from "../../wailsjs/go/main/DaemonService";

  let poll: ReturnType<typeof setInterval>;

  async function toggle() {
    try {
      const s = await SetTracking(!$daemon.running);
      daemon.set(s);
    } catch (e) {
      console.error("SetTracking failed", e);
    }
  }

  onMount(() => {
    poll = setInterval(() => {
      Status().then((s) => daemon.set(s)).catch(() => {});
    }, 5000);
  });

  onDestroy(() => clearInterval(poll));
</script>

<section>
  <header>
    <h2>{$today.date || "Today"}</h2>
    <p class="muted">
      {$today.focusMinutes} min focus · {$today.count} sessions
    </p>
    <button class="track" class:on={$daemon.running} on:click={toggle}>
      Tracking {$daemon.running ? "●" : "○"}
    </button>
  </header>

  {#if $today.sessions.length === 0}
    <p class="muted">No sessions yet today.</p>
  {:else}
    <ul>
      {#each $today.sessions as s}
        <li>
          <span class="task">{s.task}</span>
          <span class="muted">{s.minutes}m</span>
          <span class="pill {s.status}">{s.status}</span>
        </li>
      {/each}
    </ul>
  {/if}
</section>

<style>
  section {
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }
  header {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }
  header h2 {
    margin: 0;
  }
  header p {
    margin: 0;
  }
  .track {
    align-self: flex-start;
    margin-top: 0.5rem;
    background: transparent;
    border: 1px solid var(--muted);
    color: var(--fg);
    padding: 0.35rem 0.9rem;
    border-radius: 4px;
    cursor: pointer;
  }
  .track.on {
    border-color: var(--accent);
    color: var(--accent);
  }
  ul {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }
  li {
    display: flex;
    align-items: center;
    gap: 0.6rem;
  }
  .task {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .pill {
    font-size: 0.75rem;
    padding: 0.1rem 0.5rem;
    border-radius: 999px;
    border: 1px solid var(--muted);
    text-transform: capitalize;
  }
  .pill.completed {
    border-color: var(--accent);
    color: var(--accent);
  }
</style>
