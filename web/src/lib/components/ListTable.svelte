<script lang="ts">
	import { createEventDispatcher } from 'svelte';
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

<table>
	<thead>
		<tr>
			{#each keys as key}
				<th>
					<button type="button" onclick={() => dispatch('sort', { by: key })}>
						{headerLabel(key)}
						{#if sortBy === key}
							{sortDir === 'asc' ? ' ↑' : ' ↓'}
						{/if}
					</button>
				</th>
			{/each}
		</tr>
	</thead>
	<tbody>
		{#each tickets as ticket}
			<tr onclick={() => dispatch('select', { ticket })}>
				<td>{ticket.id}</td>
				<td>{ticket.title}</td>
				<td>{ticket.state}</td>
				<td>P{ticket.priority}</td>
				<td>{ticket.parent ?? '-'}</td>
				<td>{ticket.created_at}</td>
			</tr>
		{/each}
	</tbody>
</table>

<style>
	table {
		width: 100%;
		border-collapse: collapse;
		font-size: 0.9rem;
		border: 1px solid #d7e2f0;
		border-radius: 12px;
		overflow: hidden;
	}

	th,
	td {
		padding: 0.55rem 0.6rem;
		border-bottom: 1px solid #e2eaf5;
		text-align: left;
	}

	th button {
		border: 0;
		background: transparent;
		font-weight: 600;
		cursor: pointer;
	}

	tbody tr {
		cursor: pointer;
	}

	tbody tr:hover {
		background: #f5f9ff;
	}
</style>
