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
		if (priority <= 1) return 'bg-red-100/70 text-red-700 border-red-200';
		if (priority <= 2) return 'bg-amber-100/75 text-amber-700 border-amber-200';
		if (priority <= 3) return 'bg-sky-100/75 text-sky-700 border-sky-200';
		return 'bg-emerald-100/70 text-emerald-700 border-emerald-200';
	}
</script>

<button type="button" class="w-full cursor-pointer text-left" onclick={select}>
	<Card class="border-slate-200 bg-white/95 transition hover:-translate-y-0.5 hover:shadow-md">
		<CardHeader class="flex flex-row items-center justify-between gap-2 pb-2">
			<Badge variant="outline" class="bg-slate-100 text-slate-700">{ticket.id}</Badge>
			<Badge variant="outline" class={priorityTone(ticket.priority)}>P{ticket.priority}</Badge>
		</CardHeader>
		<CardContent class="space-y-3">
			<h3 class="line-clamp-2 text-sm leading-snug font-semibold text-slate-900">{ticket.title}</h3>
			{#if ticket.labels.length > 0}
				<div class="flex flex-wrap gap-1.5">
					{#each ticket.labels as label}
						<Badge variant="secondary" class="bg-slate-100/80 text-slate-700">{label}</Badge>
					{/each}
				</div>
			{/if}
		</CardContent>
	</Card>
</button>
