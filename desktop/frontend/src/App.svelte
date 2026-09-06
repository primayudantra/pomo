<script lang="ts">
  import { onMount } from "svelte";
  import { timer, initStores } from "./lib/stores";
  import QuickStart from "./screens/QuickStart.svelte";
  import Timer from "./screens/Timer.svelte";
  import Today from "./screens/Today.svelte";

  let tab: "start" | "today" = "start";

  $: phase = $timer.phase;
  $: showTimer =
    phase === "running" ||
    phase === "paused" ||
    phase === "break_prompt" ||
    phase === "break";

  onMount(initStores);
</script>

<main>
  {#if showTimer}
    <Timer />
  {:else}
    <nav>
      <button class:active={tab === "start"} on:click={() => (tab = "start")}>
        Start
      </button>
      <button class:active={tab === "today"} on:click={() => (tab = "today")}>
        Today
      </button>
    </nav>
    {#if tab === "start"}
      <QuickStart />
    {:else}
      <Today />
    {/if}
  {/if}
</main>

<style>
  main {
    padding: 1.5rem;
  }
  nav {
    display: flex;
    gap: 0.5rem;
    margin-bottom: 1rem;
  }
  nav button {
    background: transparent;
    border: 1px solid var(--muted);
    color: var(--fg);
    padding: 0.35rem 0.9rem;
    border-radius: 4px;
    cursor: pointer;
  }
  nav button.active {
    border-color: var(--accent);
    color: var(--accent);
  }
</style>
