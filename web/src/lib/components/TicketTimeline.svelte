<script lang="ts">
	import HumanDateTime from '$lib/components/HumanDateTime.svelte';
	import type { Ticket } from '$lib/types';

	type TicketEvent = {
		id: string;
		label: string;
		at: string;
		accent: 'neutral' | 'active' | 'done';
	};

	let { ticket } = $props<{ ticket: Ticket }>();

	function parseEventTime(raw: string): number {
		const parsed = new Date(raw).getTime();
		return Number.isNaN(parsed) ? Number.MAX_SAFE_INTEGER : parsed;
	}

	const events = $derived.by(() => {
		const next: TicketEvent[] = [];
		if (ticket.created_at) {
			next.push({ id: 'created', label: 'Created', at: ticket.created_at, accent: 'neutral' });
		}
		if (ticket.started_at) {
			next.push({ id: 'started', label: 'Started', at: ticket.started_at, accent: 'active' });
		}
		if (ticket.completed_at) {
			next.push({ id: 'completed', label: 'Completed', at: ticket.completed_at, accent: 'done' });
		}
		if (
			ticket.updated_at &&
			ticket.updated_at !== ticket.created_at &&
			ticket.updated_at !== ticket.started_at &&
			ticket.updated_at !== ticket.completed_at
		) {
			next.push({ id: 'updated', label: 'Last Updated', at: ticket.updated_at, accent: 'neutral' });
		}

		return next.sort((left, right) => parseEventTime(left.at) - parseEventTime(right.at));
	});

	function markerClass(accent: TicketEvent['accent']): string {
		if (accent === 'done') return 'bg-emerald-500';
		if (accent === 'active') return 'bg-blue-500';
		return 'bg-border';
	}
</script>

<ol class="space-y-4">
	{#each events as event, idx (event.id)}
		<li class="flex items-start gap-3">
			<div class="relative flex w-3 justify-center">
				<span class={`mt-1 h-2.5 w-2.5 rounded-full ${markerClass(event.accent)}`}></span>
				{#if idx < events.length - 1}
					<span class="absolute top-4 bottom-[-1rem] w-px bg-border"></span>
				{/if}
			</div>
			<div class="min-w-0 flex-1">
				<p class="text-[11px] font-semibold tracking-wide text-muted-foreground uppercase">{event.label}</p>
				<HumanDateTime value={event.at} />
			</div>
		</li>
	{/each}
</ol>
