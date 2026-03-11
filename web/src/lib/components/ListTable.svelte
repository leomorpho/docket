<script lang="ts">
	import { createEventDispatcher } from 'svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Card, CardContent } from '$lib/components/ui/card';
	import {
		Table,
		TableBody,
		TableCell,
		TableHead,
		TableHeader,
		TableRow
	} from '$lib/components/ui/table';
	import type { Ticket } from '$lib/types';

	type SortKey = 'id' | 'title' | 'state' | 'priority' | 'parent' | 'created_at';
	const keys: SortKey[] = ['id', 'title', 'state', 'priority', 'parent', 'created_at'];

	let { tickets, sortBy, sortDir } = $props<{
		tickets: Ticket[];
		sortBy: SortKey;
		sortDir: 'asc' | 'desc';
	}>();

	const dispatch = createEventDispatcher<{
		sort: { by: SortKey };
		select: { ticket: Ticket };
	}>();

	function headerLabel(k: SortKey): string {
		return (
			{
				id: 'ID',
				title: 'Title',
				state: 'State',
				priority: 'Priority',
				parent: 'Parent',
				created_at: 'Created'
			}[k] ?? k
		);
	}
</script>

<Card class="border-slate-200 bg-white/90 shadow-sm">
	<CardContent class="p-0">
		<Table>
			<TableHeader>
				<TableRow class="bg-slate-50/70 hover:bg-slate-50/70">
					{#each keys as key}
						<TableHead>
							<button
								type="button"
								class="inline-flex w-full cursor-pointer items-center gap-1 text-left text-xs font-semibold tracking-wide uppercase text-slate-600"
								onclick={() => dispatch('sort', { by: key })}
							>
								{headerLabel(key)}
								{#if sortBy === key}
									<span aria-hidden="true">{sortDir === 'asc' ? '↑' : '↓'}</span>
								{/if}
							</button>
						</TableHead>
					{/each}
				</TableRow>
			</TableHeader>
			<TableBody>
				{#each tickets as ticket}
					<TableRow
						class="cursor-pointer bg-white/70 transition hover:bg-slate-50"
						onclick={() => dispatch('select', { ticket })}
					>
						<TableCell class="font-medium text-slate-700">{ticket.id}</TableCell>
						<TableCell class="max-w-[36ch] truncate">{ticket.title}</TableCell>
						<TableCell><Badge variant="outline">{ticket.state}</Badge></TableCell>
						<TableCell><Badge variant="secondary">P{ticket.priority}</Badge></TableCell>
						<TableCell>{ticket.parent ?? '-'}</TableCell>
						<TableCell class="text-muted-foreground">{ticket.created_at}</TableCell>
					</TableRow>
				{/each}
			</TableBody>
		</Table>
	</CardContent>
</Card>
