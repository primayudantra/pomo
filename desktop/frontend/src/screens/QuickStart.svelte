<script lang="ts">
  import { Start } from "../../wailsjs/go/main/TimerService";

  let task = "";
  let minutes = 25;
  let error = "";
  const chips = [15, 25, 50];

  async function start() {
    if (!task.trim()) return;
    try {
      await Start(task.trim(), minutes);
      error = "";
    } catch (e) {
      error = String(e);
    }
  }
</script>

<section>
  <h2>Start</h2>
  <!-- svelte-ignore a11y-autofocus -->
  <input
    placeholder="What are you working on?"
    bind:value={task}
    on:keydown={(e) => e.key === "Enter" && start()}
    autofocus
  />
  <div class="chips">
    {#each chips as c}
      <button class:active={minutes === c} on:click={() => (minutes = c)}>
        {c}m
      </button>
    {/each}
  </div>
  {#if error}<p class="err">{error}</p>{/if}
  <button class="primary" on:click={start}>Start</button>
</section>

<style>
  section {
    display: flex;
    flex-direction: column;
    gap: 1rem;
    max-width: 28rem;
  }
  input {
    background: transparent;
    border: 1px solid var(--muted);
    color: var(--fg);
    padding: 0.6rem 0.75rem;
    border-radius: 4px;
    font-size: 1rem;
  }
  input:focus {
    outline: none;
    border-color: var(--accent);
  }
  .chips {
    display: flex;
    gap: 0.5rem;
  }
  .chips button {
    background: transparent;
    border: 1px solid var(--muted);
    color: var(--fg);
    padding: 0.4rem 0.9rem;
    border-radius: 4px;
    cursor: pointer;
  }
  .chips button.active {
    border-color: var(--accent);
    color: var(--accent);
  }
  .primary {
    background: var(--accent);
    border: none;
    color: #fff;
    padding: 0.6rem 1rem;
    border-radius: 4px;
    cursor: pointer;
    font-size: 1rem;
    align-self: flex-start;
  }
  .err {
    color: var(--accent);
    margin: 0;
  }
</style>
