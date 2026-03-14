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
		'bg-muted/30',
		'bg-muted/25',
		'bg-muted/35',
		'bg-muted/20',
		'bg-muted/30'
	];
</script>

<section class="flex gap-3 overflow-x-auto pb-2">
	{#each columns as col, idx}
		<Card
			class={`min-h-[60vh] min-w-[300px] flex-1 border-border shadow-sm ${columnBackgrounds[idx % columnBackgrounds.length]}`}
		>
			<CardHeader class="flex flex-row items-center justify-between gap-2 border-b border-border/80 pb-3">
				<CardTitle class="text-sm font-semibold tracking-wide uppercase text-foreground">{col.label}</CardTitle>
				<Badge variant="outline" class="bg-background/85 text-foreground">{col.tickets.length}</Badge>
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
