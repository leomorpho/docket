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

	let { tickets, sortBy, sortDir, childCounts = {} } = $props<{
		tickets: Ticket[];
		sortBy: SortKey;
		sortDir: 'asc' | 'desc';
		childCounts?: Record<string, number>;
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

	function formatLocalDateTime(value: string): string {
		const parsed = new Date(value);
		if (Number.isNaN(parsed.getTime())) return value;
		return parsed.toLocaleString();
	}
</script>

<Card class="border-border bg-card/90 shadow-sm">
	<CardContent class="p-0">
		<Table>
			<TableHeader>
				<TableRow class="bg-muted/30 hover:bg-muted/30">
					{#each keys as key}
						<TableHead>
							<button
								type="button"
								class="inline-flex w-full cursor-pointer items-center gap-1 text-left text-xs font-semibold tracking-wide uppercase text-muted-foreground"
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
						class="cursor-pointer bg-card/70 transition hover:bg-muted/30"
						onclick={() => dispatch('select', { ticket })}
					>
						<TableCell class="font-medium text-foreground">{ticket.id}</TableCell>
						<TableCell class="max-w-[48ch]">
							<p class="truncate text-foreground">{ticket.title}</p>
							{#if ticket.parent || (childCounts?.[ticket.id] ?? 0) > 0}
								<div class="mt-1 flex flex-wrap items-center gap-1">
									{#if ticket.parent}
										<Badge variant="outline" class="text-[10px]">parent {ticket.parent}</Badge>
									{/if}
									{#if (childCounts?.[ticket.id] ?? 0) > 0}
										<Badge variant="secondary" class="text-[10px]">
											{childCounts?.[ticket.id]} child{(childCounts?.[ticket.id] ?? 0) === 1 ? '' : 'ren'}
										</Badge>
									{/if}
								</div>
							{/if}
						</TableCell>
						<TableCell><Badge variant="outline">{ticket.state}</Badge></TableCell>
						<TableCell><Badge variant="secondary">P{ticket.priority}</Badge></TableCell>
						<TableCell>{ticket.parent ?? '-'}</TableCell>
						<TableCell class="text-muted-foreground">{formatLocalDateTime(ticket.created_at)}</TableCell>
					</TableRow>
				{/each}
			</TableBody>
		</Table>
	</CardContent>
</Card>
