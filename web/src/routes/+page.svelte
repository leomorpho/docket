<script lang="ts">
	import BoardView from '$lib/components/BoardView.svelte';
	import DetailSheet from '$lib/components/DetailSheet.svelte';
	import FilterBar from '$lib/components/FilterBar.svelte';
	import ListTable from '$lib/components/ListTable.svelte';
	import type { Config, StateConfig, Ticket } from '$lib/types';

	let { data } = $props<{ data: { config: Config; tickets: Ticket[] } }>();

	const openStateEntries = (Object.entries(data.config.states) as [string, StateConfig][])
		.filter(([, st]) => st.open)
		.sort((a, b) => a[1].column - b[1].column);

	const openStates = openStateEntries.map(([key]) => key);
	const stateOptions = openStateEntries.map(([key, st]) => ({ key, label: st.label }));
	const labelOptions: string[] = Array.from(new Set(data.tickets.flatMap((t: Ticket) => t.labels))).sort() as string[];

	type SortKey = 'id' | 'title' | 'state' | 'priority' | 'parent' | 'created_at';
	let selectedStates = $state(new Set(openStates));
	let selectedLabel = $state('');
	let maxPriority = $state(0);
	let mode = $state<'board' | 'list'>('board');
	let selectedTicket = $state<Ticket | null>(null);
	let sheetOpen = $state(false);
	let sortBy = $state<SortKey>('priority');
	let sortDir = $state<'asc' | 'desc'>('asc');

	const filtered = $derived(data.tickets.filter((t: Ticket) => {
		if (!selectedStates.has(t.state)) return false;
		if (selectedLabel && !t.labels.includes(selectedLabel)) return false;
		if (maxPriority > 0 && t.priority > maxPriority) return false;
		return true;
	}));

	const columns = $derived(openStateEntries.map(([key, st]) => ({
		key,
		label: st.label,
		column: st.column,
		tickets: filtered.filter((t: Ticket) => t.state === key).sort((a: Ticket, b: Ticket) => a.priority - b.priority || a.seq - b.seq)
	})));

	const sortedList = $derived([...filtered].sort((a, b) => {
		const av = a[sortBy] ?? '';
		const bv = b[sortBy] ?? '';
		if (av < bv) return sortDir === 'asc' ? -1 : 1;
		if (av > bv) return sortDir === 'asc' ? 1 : -1;
		return 0;
	}));

	function onCardSelect(ticket: Ticket) {
		selectedTicket = ticket;
		sheetOpen = true;
	}

	function toggleState(key: string) {
		if (selectedStates.has(key)) selectedStates.delete(key);
		else selectedStates.add(key);
		selectedStates = new Set(selectedStates);
	}

	function clearFilters() {
		selectedStates = new Set(openStates);
		selectedLabel = '';
		maxPriority = 0;
	}

	function toggleSort(by: SortKey) {
		if (sortBy === by) {
			sortDir = sortDir === 'asc' ? 'desc' : 'asc';
			return;
		}
		sortBy = by;
		sortDir = 'asc';
	}
</script>

<main>
	<header>
		<div>
			<h1>Docket UI</h1>
			<p>{filtered.length} of {data.tickets.length} tickets</p>
		</div>
		<div class="modes">
			<button type="button" class:active={mode === 'board'} onclick={() => (mode = 'board')}>Board</button>
			<button type="button" class:active={mode === 'list'} onclick={() => (mode = 'list')}>List</button>
		</div>
	</header>

	<FilterBar
		{stateOptions}
		{labelOptions}
		{selectedStates}
		{selectedLabel}
		{maxPriority}
		on:toggleState={(e: CustomEvent<{ key: string }>) => toggleState(e.detail.key)}
		on:label={(e: CustomEvent<{ value: string }>) => (selectedLabel = e.detail.value)}
		on:priority={(e: CustomEvent<{ value: number }>) => (maxPriority = e.detail.value)}
		on:clear={clearFilters}
	/>

	{#if mode === 'board'}
		<BoardView {columns} on:select={(e: CustomEvent<{ ticket: Ticket }>) => onCardSelect(e.detail.ticket)} />
	{:else}
		<ListTable
			tickets={sortedList}
			{sortBy}
			{sortDir}
			on:sort={(e: CustomEvent<{ by: SortKey }>) => toggleSort(e.detail.by)}
			on:select={(e: CustomEvent<{ ticket: Ticket }>) => onCardSelect(e.detail.ticket)}
		/>
	{/if}

	<DetailSheet ticket={selectedTicket} open={sheetOpen} onClose={() => (sheetOpen = false)} />
</main>

<style>
	main {
		padding: 1rem;
		display: flex;
		flex-direction: column;
		gap: 0.9rem;
	}

	header {
		display: flex;
		justify-content: space-between;
		align-items: flex-end;
		gap: 0.8rem;
	}

	h1 {
		margin: 0;
		font-size: 1.4rem;
	}

	p {
		margin: 0.2rem 0 0;
		color: #51627f;
	}

	.modes {
		display: inline-flex;
		border: 1px solid #cfdcec;
		border-radius: 10px;
		overflow: hidden;
	}

	.modes button {
		border: 0;
		padding: 0.35rem 0.7rem;
		background: #fff;
		cursor: pointer;
	}

	.modes button.active {
		background: #e6eefb;
	}
</style>
