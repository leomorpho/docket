<script lang="ts">
	import { createEventDispatcher } from 'svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Card, CardContent, CardHeader } from '$lib/components/ui/card';
	import type { Ticket } from '$lib/types';

	let { ticket } = $props<{ ticket: Ticket }>();
	const dispatch = createEventDispatcher<{ select: { ticket: Ticket } }>();

	function select() {
		dispatch('select', { ticket });
	}

	function priorityTone(priority: number): string {
		if (priority <= 1) return 'bg-destructive/15 text-destructive border-destructive/30';
		if (priority <= 2) return 'bg-chart-5/20 text-chart-5 border-chart-5/30';
		if (priority <= 3) return 'bg-chart-2/20 text-chart-2 border-chart-2/30';
		return 'bg-chart-4/20 text-chart-4 border-chart-4/30';
	}
</script>

<button type="button" class="w-full cursor-pointer text-left" onclick={select}>
	<Card class="border-border bg-card/95 transition hover:-translate-y-0.5 hover:shadow-md">
		<CardHeader class="flex flex-row items-center justify-between gap-2 pb-2">
			<Badge variant="outline" class="bg-muted text-foreground">{ticket.id}</Badge>
			<Badge variant="outline" class={priorityTone(ticket.priority)}>P{ticket.priority}</Badge>
		</CardHeader>
		<CardContent class="space-y-3">
			<h3 class="line-clamp-2 text-sm leading-snug font-semibold text-foreground">{ticket.title}</h3>
			{#if ticket.labels.length > 0}
				<div class="flex flex-wrap gap-1.5">
					{#each ticket.labels as label}
						<Badge variant="secondary" class="bg-muted/80 text-foreground">{label}</Badge>
					{/each}
				</div>
			{/if}
		</CardContent>
	</Card>
</button>
