<script lang="ts">
	import { createEventDispatcher } from 'svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Card, CardContent, CardHeader, CardTitle } from '$lib/components/ui/card';
	import { ScrollArea } from '$lib/components/ui/scroll-area';
	import TicketCard from '$lib/components/TicketCard.svelte';
	import type { Ticket } from '$lib/types';

	type Column = {
		key: string;
		label: string;
		column: number;
		tickets: Ticket[];
	};

	let { columns } = $props<{ columns: Column[] }>();
	const dispatch = createEventDispatcher<{ select: { ticket: Ticket } }>();

	const columnBackgrounds = [
		'bg-sky-50/75',
		'bg-emerald-50/70',
		'bg-amber-50/75',
		'bg-rose-50/70',
		'bg-indigo-50/70'
	];
</script>

<section class="flex gap-3 overflow-x-auto pb-2">
	{#each columns as col, idx}
		<Card
			class={`min-h-[60vh] min-w-[300px] flex-1 border-slate-200 shadow-sm ${columnBackgrounds[idx % columnBackgrounds.length]}`}
		>
			<CardHeader class="flex flex-row items-center justify-between gap-2 border-b border-slate-200/80 pb-3">
				<CardTitle class="text-sm font-semibold tracking-wide uppercase text-slate-700">{col.label}</CardTitle>
				<Badge variant="outline" class="bg-white/85 text-slate-700">{col.tickets.length}</Badge>
			</CardHeader>
			<CardContent class="px-3 pb-3">
				<ScrollArea class="h-[65vh]">
					<div class="flex flex-col gap-2 pr-2">
						{#each col.tickets as ticket}
							<TicketCard
								{ticket}
								on:select={(e: CustomEvent<{ ticket: Ticket }>) => dispatch('select', e.detail)}
							/>
						{/each}
					</div>
				</ScrollArea>
			</CardContent>
		</Card>
	{/each}
</section>
