<script lang="ts">
  import { timer } from "../lib/stores";
  import { mmss } from "../lib/format";
  // Stub — Task 11 fills this in.
</script>

<section>
  <h2>{$timer.task || "Focus"}</h2>
  <div class="countdown">{mmss($timer.remaining)}</div>
  <p class="muted">{$timer.phase}</p>
</section>
