<script lang="ts">
	import { createEventDispatcher } from 'svelte';
	import { Badge } from '$lib/components/ui/badge';
	import type { Ticket } from '$lib/types';

	let { id, ticket = null, compact = false } = $props<{
		id: string;
		ticket?: Ticket | null;
		compact?: boolean;
	}>();

	const dispatch = createEventDispatcher<{ select: { id: string } }>();

	function select() {
		dispatch('select', { id });
	}
</script>

<div class="group relative inline-block align-top">
	<button
		type="button"
		class={`cursor-pointer rounded-md border border-border bg-background font-mono text-xs text-foreground shadow-xs transition-colors hover:bg-accent/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/40 ${
			compact ? 'h-6 px-2 text-[10px]' : 'h-7 px-2'
		}`}
		onclick={select}
		aria-label={`Open ${id}`}
		title={`Open ${id}`}
	>
		{id}
	</button>

	<div class="pointer-events-none absolute top-[calc(100%+0.35rem)] left-0 z-50 w-72 translate-y-1 rounded-lg border border-border bg-card p-3 opacity-0 shadow-lg transition-all duration-150 group-hover:translate-y-0 group-hover:opacity-100 group-focus-within:translate-y-0 group-focus-within:opacity-100">
		<div class="space-y-2">
			<div class="flex items-center justify-between gap-2">
				<p class="font-mono text-xs font-semibold text-muted-foreground">{id}</p>
				{#if ticket}
					<div class="flex items-center gap-1">
						<Badge variant="outline" class="text-[10px]">{ticket.state}</Badge>
						<Badge variant="secondary" class="text-[10px]">P{ticket.priority}</Badge>
					</div>
				{/if}
			</div>

			<p class="line-clamp-2 text-sm font-medium text-foreground">
				{ticket?.title ?? 'Ticket metadata unavailable in current view.'}
			</p>

			{#if ticket}
				<div class="flex flex-wrap gap-1">
					{#if ticket.parent}
						<Badge variant="outline" class="text-[10px]">parent {ticket.parent}</Badge>
					{/if}
					{#if (ticket.labels?.length ?? 0) > 0}
						{#each ticket.labels.slice(0, 3) as label}
							<Badge variant="secondary" class="text-[10px]">{label}</Badge>
						{/each}
						{#if ticket.labels.length > 3}
							<Badge variant="outline" class="text-[10px]">+{ticket.labels.length - 3} more</Badge>
						{/if}
					{/if}
				</div>
			{/if}

			<p class="text-[11px] text-muted-foreground">Click to open this ticket.</p>
		</div>
	</div>
</div>
