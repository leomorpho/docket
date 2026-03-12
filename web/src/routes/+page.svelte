<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import BoardView from '$lib/components/BoardView.svelte';
	import DetailSheet from '$lib/components/DetailSheet.svelte';
	import FilterBar from '$lib/components/FilterBar.svelte';
	import ListTable from '$lib/components/ListTable.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Card, CardContent, CardHeader, CardTitle } from '$lib/components/ui/card';
	import { Tabs, TabsContent, TabsList, TabsTrigger } from '$lib/components/ui/tabs';
	import type { Config, StateConfig, Ticket } from '$lib/types';

	let { data } = $props<{ data: { config: Config; tickets: Ticket[] } }>();

	const openStateEntries = $derived.by(() =>
		(Object.entries(data.config.states) as [string, StateConfig][])
			.filter(([, st]) => st.open)
			.sort((a, b) => a[1].column - b[1].column)
	);

	const openStates = $derived(openStateEntries.map(([key]) => key));
	const allStates = $derived.by(() =>
		(Object.entries(data.config.states) as [string, StateConfig][])
			.sort((a, b) => a[1].column - b[1].column)
			.map(([key, st]) => ({ key, label: st.label }))
	);
	const stateOptions = $derived(openStateEntries.map(([key, st]) => ({ key, label: st.label })));
	const labelOptions: string[] = $derived(
		Array.from(new Set(data.tickets.flatMap((t: Ticket) => t.labels))).sort() as string[]
	);

	type SortKey = 'id' | 'title' | 'state' | 'priority' | 'parent' | 'created_at';
	let selectedStates = $state(new Set<string>());
	let selectedStatesInitialized = $state(false);
	let selectedLabel = $state('');
	let maxPriority = $state(0);
	let mode = $state<'board' | 'list'>('board');
	let selectedTicket = $state<Ticket | null>(null);
	let sheetOpen = $state(false);
	let sortBy = $state<SortKey>('priority');
	let sortDir = $state<'asc' | 'desc'>('asc');

	$effect(() => {
		if (!selectedStatesInitialized && openStates.length > 0) {
			selectedStates = new Set(openStates);
			selectedStatesInitialized = true;
		}
	});

	$effect(() => {
		if (!selectedTicket) return;
		const refreshed = data.tickets.find((t: Ticket) => t.id === selectedTicket?.id) ?? null;
		selectedTicket = refreshed;
	});

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

	type MutationKind = 'state' | 'title' | 'desc';
	type MutationResult = { ok: boolean; error?: string };

	async function mutateTicket(ticketID: string, kind: MutationKind, value: string): Promise<MutationResult> {
		const response = await fetch(`/api/tickets/${ticketID}`, {
			method: 'PATCH',
			headers: { 'content-type': 'application/json' },
			body: JSON.stringify({ kind, value })
		});
		const payload = (await response.json().catch(() => ({}))) as MutationResult;
		if (!response.ok || !payload.ok) {
			return { ok: false, error: payload.error ?? 'Ticket update failed.' };
		}
		await invalidateAll();
		return { ok: true };
	}

	function updateState(ticketID: string, value: string) {
		return mutateTicket(ticketID, 'state', value);
	}

	function updateTitle(ticketID: string, value: string) {
		return mutateTicket(ticketID, 'title', value);
	}

	function updateDescription(ticketID: string, value: string) {
		return mutateTicket(ticketID, 'desc', value);
	}
</script>

<main class="min-h-screen bg-gradient-to-br from-slate-50 via-zinc-50 to-slate-100 px-4 py-5 sm:px-6">
	<div class="mx-auto flex w-full max-w-[1400px] flex-col gap-4">
		<Card class="border-slate-200/80 bg-white/90 shadow-sm">
			<CardHeader class="flex flex-row items-start justify-between gap-3">
				<div class="space-y-1">
					<CardTitle class="text-2xl tracking-tight">Docket UI</CardTitle>
					<p class="text-sm text-muted-foreground">
						{filtered.length} of {data.tickets.length} tickets visible
					</p>
				</div>
				<Badge variant="secondary">{openStates.length} workflow states</Badge>
			</CardHeader>
			<CardContent>
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
			</CardContent>
		</Card>

		<Tabs bind:value={mode} class="gap-3">
			<TabsList class="w-fit border bg-white/80 p-1 shadow-xs">
				<TabsTrigger value="board">Board</TabsTrigger>
				<TabsTrigger value="list">List</TabsTrigger>
			</TabsList>

			<TabsContent value="board" class="mt-0">
				<BoardView
					{columns}
					on:select={(e: CustomEvent<{ ticket: Ticket }>) => onCardSelect(e.detail.ticket)}
				/>
			</TabsContent>

			<TabsContent value="list" class="mt-0">
				<ListTable
					tickets={sortedList}
					{sortBy}
					{sortDir}
					on:sort={(e: CustomEvent<{ by: SortKey }>) => toggleSort(e.detail.by)}
					on:select={(e: CustomEvent<{ ticket: Ticket }>) => onCardSelect(e.detail.ticket)}
				/>
			</TabsContent>
		</Tabs>
	</div>

	<DetailSheet
		ticket={selectedTicket}
		bind:open={sheetOpen}
		stateOptions={allStates}
		onUpdateState={updateState}
		onUpdateTitle={updateTitle}
		onUpdateDescription={updateDescription}
	/>
</main>
