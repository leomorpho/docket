<script lang="ts">
	import { tick } from 'svelte';

	let { searchQuery = $bindable('') } = $props<{
		searchQuery?: string;
	}>();

	let searchInput = $state<HTMLInputElement | null>(null);

	export async function focusSearch() {
		await tick();
		searchInput?.focus();
	}
</script>

<div class="rounded-xl border border-border bg-muted/40 p-3">
	<input
		bind:this={searchInput}
		type="text"
		placeholder="Search tickets by id, title, or description... (/)"
		class="h-9 w-full rounded-md border border-input bg-background px-3 text-sm focus:outline-none focus:ring-2 focus:ring-ring/40"
		bind:value={searchQuery}
	/>
</div>
