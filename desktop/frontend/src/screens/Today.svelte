<script lang="ts">
  import { today } from "../lib/stores";
  // Stub — Task 11 fills this in.
</script>

<section>
  <h2>Today</h2>
  <p class="muted">
    {$today.count} sessions · {$today.focusMinutes} min focused
  </p>
  <ul>
    {#each $today.sessions as s}
      <li>{s.task} — {s.minutes}m ({s.status})</li>
    {/each}
  </ul>
</section>
