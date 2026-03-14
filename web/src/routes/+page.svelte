<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import BoardView from '$lib/components/BoardView.svelte';
	import CreateTicketModal from '$lib/components/CreateTicketModal.svelte';
	import DetailSheet from '$lib/components/DetailSheet.svelte';
	import FilterBar from '$lib/components/FilterBar.svelte';
	import HealthView from '$lib/components/HealthView.svelte';
	import ListTable from '$lib/components/ListTable.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Card, CardContent, CardHeader, CardTitle } from '$lib/components/ui/card';
	import { Tabs, TabsContent, TabsList, TabsTrigger } from '$lib/components/ui/tabs';
	import type { Config, Project, StateConfig, Ticket } from '$lib/types';
	import { onMount } from 'svelte';

	let { data } = $props<{
		data: {
			projects: Project[];
			activeProjectId: string | null;
			config: Config;
			tickets: Ticket[];
		};
	}>();
	let showClosedStates = $state(false);

	const openStateEntries = $derived.by(() =>
		(Object.entries(data.config.states) as [string, StateConfig][])
			.filter(([, st]) => st.open)
			.sort((a, b) => a[1].column - b[1].column)
	);
	const allStateEntries = $derived.by(() =>
		(Object.entries(data.config.states) as [string, StateConfig][])
			.sort((a, b) => a[1].column - b[1].column)
	);
	const visibleStateEntries = $derived(showClosedStates ? allStateEntries : openStateEntries);

	const openStates = $derived(openStateEntries.map(([key]) => key));
	const allStates = $derived.by(() =>
		(Object.entries(data.config.states) as [string, StateConfig][])
			.sort((a, b) => a[1].column - b[1].column)
			.map(([key, st]) => ({ key, label: st.label }))
	);
	const visibleStates = $derived(visibleStateEntries.map(([key]) => key));
	const visibleStateKey = $derived(visibleStates.join('|'));
	const stateOptions = $derived(visibleStateEntries.map(([key, st]) => ({ key, label: st.label })));
	const labelOptions: string[] = $derived(
		Array.from(new Set(data.tickets.flatMap((t: Ticket) => t.labels))).sort() as string[]
	);

	const activeProject = $derived(
		data.projects.find((p: Project) => p.id === data.activeProjectId) ?? data.projects[0] ?? null
	);

	type SortKey = 'id' | 'title' | 'state' | 'priority' | 'parent' | 'created_at';
	type MutationResult = { ok: boolean; error?: string };
	type StateUpdateOptions = { approvalTicket?: string; confirmed?: boolean };
	let selectedStates = $state(new Set<string>());
	let selectedStatesKey = $state('');
	let selectedLabel = $state('');
	let maxPriority = $state(0);
	let mode = $state<'board' | 'list' | 'review' | 'health'>('board');
	let selectedTicket = $state<Ticket | null>(null);
	let sheetOpen = $state(false);
	let sortBy = $state<SortKey>('priority');
	let sortDir = $state<'asc' | 'desc'>('asc');
	let searchQuery = $state('');
	let filterBar = $state<ReturnType<typeof FilterBar> | null>(null);
	let createModalOpen = $state(false);
	let secureActive = $state(false);
	let secureExpiresAt = $state('');
	let secureStatusError = $state('');

	onMount(() => {
		const handleKeydown = (e: KeyboardEvent) => {
			if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) {
				if (e.key === 'Escape') {
					(e.target as HTMLElement).blur();
				}
				return;
			}

			if (e.key === 'b') mode = 'board';
			if (e.key === 'l') mode = 'list';
			if (e.key === 'h') mode = 'health';
			if (e.key === 'n') {
				e.preventDefault();
				createModalOpen = true;
			}
			if (e.key === '/') {
				e.preventDefault();
				filterBar?.focusSearch();
			}
			if (e.key === 'Escape') {
				if (sheetOpen) sheetOpen = false;
				if (createModalOpen) createModalOpen = false;
			}
		};
		window.addEventListener('keydown', handleKeydown);

		const persisted = localStorage.getItem('docket_active_project');
		if (persisted && !data.activeProjectId && data.projects.some((p: Project) => p.id === persisted)) {
			switchProject(persisted);
		}

		return () => window.removeEventListener('keydown', handleKeydown);
	});

	$effect(() => {
		if (data.activeProjectId) {
			localStorage.setItem('docket_active_project', data.activeProjectId);
		}
	});

	$effect(() => {
		void refreshSecureStatus();
	});

	$effect(() => {
		if (visibleStateKey && visibleStateKey !== selectedStatesKey) {
			selectedStates = new Set(visibleStates);
			selectedStatesKey = visibleStateKey;
		}
	});

	// Handle initial ticket from URL
	$effect(() => {
		const ticketId = $page.url.searchParams.get('ticket');
		if (ticketId && !selectedTicket) {
			const t = data.tickets.find((t: Ticket) => t.id === ticketId);
			if (t) {
				selectedTicket = t;
				sheetOpen = true;
				// Ensure its state is selected so it's visible if needed
				if (!selectedStates.has(t.state)) {
					selectedStates.add(t.state);
					selectedStates = new Set(selectedStates);
				}
			}
		}
	});

	$effect(() => {
		if (!sheetOpen && $page.url.searchParams.has('ticket')) {
			const url = new URL($page.url);
			url.searchParams.delete('ticket');
			goto(url.toString(), { replaceState: true, noScroll: true, keepFocus: true });
		}
	});

	// Reset filters when project changes
	$effect(() => {
		if (data.activeProjectId) {
			selectedStates = new Set(visibleStates);
			selectedStatesKey = visibleStateKey;
			selectedLabel = '';
			maxPriority = 0;
			selectedTicket = null;
			sheetOpen = false;
		}
	});

	$effect(() => {
		if (!selectedTicket) return;
		const refreshed = data.tickets.find((t: Ticket) => t.id === selectedTicket?.id) ?? null;
		selectedTicket = refreshed;
	});

	const filtered = $derived(
		data.tickets.filter((t: Ticket) => {
			if (!selectedStates.has(t.state)) return false;
			if (selectedLabel && !t.labels.includes(selectedLabel)) return false;
			if (maxPriority > 0 && t.priority > maxPriority) return false;
			if (searchQuery) {
				const q = searchQuery.toLowerCase();
				return (
					t.id.toLowerCase().includes(q) ||
					t.title.toLowerCase().includes(q) ||
					t.body.toLowerCase().includes(q)
				);
			}
			return true;
		})
	);

	const columns = $derived(visibleStateEntries.map(([key, st]) => ({
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
	const reviewQueue = $derived(
		[...data.tickets]
			.filter((t: Ticket) => t.state === 'in-review' || t.state === 'done')
			.sort((a: Ticket, b: Ticket) => {
				const rank = (state: string) => (state === 'in-review' ? 0 : 1);
				if (rank(a.state) !== rank(b.state)) return rank(a.state) - rank(b.state);
				if (a.priority !== b.priority) return a.priority - b.priority;
				return a.seq - b.seq;
			})
	);

	function switchProject(id: string) {
		const url = new URL($page.url);
		url.searchParams.set('project', id);
		goto(url.toString(), { invalidateAll: true });
	}

	async function addProject() {
		const dir = prompt('Enter the absolute path to your Docket repository:');
		if (!dir) return;
		try {
			const response = await fetch('/api/projects', {
				method: 'POST',
				headers: { 'content-type': 'application/json' },
				body: JSON.stringify({ dir })
			});
			const result = await response.json();
			if (result.ok) {
				switchProject(result.project.id);
			} else {
				alert(result.error || 'Failed to register project.');
			}
		} catch (e: any) {
			alert(e.message);
		}
	}

	function onCardSelect(ticket: Ticket) {
		selectedTicket = ticket;
		sheetOpen = true;
		const url = new URL($page.url);
		url.searchParams.set('ticket', ticket.id);
		goto(url.toString(), { replaceState: true, noScroll: true, keepFocus: true });
	}

	function toggleState(key: string) {
		if (selectedStates.has(key)) selectedStates.delete(key);
		else selectedStates.add(key);
		selectedStates = new Set(selectedStates);
	}

	function clearFilters() {
		selectedStates = new Set(visibleStates);
		selectedLabel = '';
		maxPriority = 0;
	}

	function applyStatePreset(states: string[]) {
		selectedStates = new Set(states);
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

	async function refreshSecureStatus() {
		const query = data.activeProjectId ? `?projectId=${encodeURIComponent(data.activeProjectId)}` : '';
		try {
			const response = await fetch(`/api/secure/status${query}`);
			const payload = await response.json().catch(() => ({}));
			if (!response.ok || !payload.ok || !payload.secure) {
				secureActive = false;
				secureExpiresAt = '';
				secureStatusError = payload.error ?? 'Unable to read secure mode status.';
				return;
			}
			secureActive = Boolean(payload.secure.active);
			secureExpiresAt = typeof payload.secure.expiresAt === 'string' ? payload.secure.expiresAt : '';
			secureStatusError = typeof payload.secure.error === 'string' ? payload.secure.error : '';
		} catch (err: any) {
			secureActive = false;
			secureExpiresAt = '';
			secureStatusError = err?.message ?? 'Unable to read secure mode status.';
		}
	}

	async function mutateTicket(
		ticketID: string,
		kind: MutationKind,
		value: string,
		options?: StateUpdateOptions
	): Promise<MutationResult> {
		const privileged = kind === 'state' && (value === 'done' || value === 'archived');
		const endpoint = privileged ? `/api/tickets/${ticketID}/privileged` : `/api/tickets/${ticketID}`;
		const method = privileged ? 'POST' : 'PATCH';
		const body = privileged
			? {
					state: value,
					approvalTicket: options?.approvalTicket ?? ticketID,
					confirm: Boolean(options?.confirmed),
					projectId: data.activeProjectId
				}
			: { kind, value, projectId: data.activeProjectId };
		const response = await fetch(endpoint, {
			method,
			headers: { 'content-type': 'application/json' },
			body: JSON.stringify(body)
		});
		const payload = (await response.json().catch(() => ({}))) as MutationResult;
		if (!response.ok || !payload.ok) {
			return { ok: false, error: payload.error ?? 'Ticket update failed.' };
		}
		const url = new URL($page.url);
		if (data.activeProjectId) url.searchParams.set('project', data.activeProjectId);
		await goto(url.toString(), { invalidateAll: true });
		await refreshSecureStatus();
		return { ok: true };
	}

	function updateState(ticketID: string, value: string, options?: StateUpdateOptions) {
		return mutateTicket(ticketID, 'state', value, options);
	}

	function updateTitle(ticketID: string, value: string) {
		return mutateTicket(ticketID, 'title', value);
	}

	function updateDescription(ticketID: string, value: string) {
		return mutateTicket(ticketID, 'desc', value);
	}

	async function updateAC(ticketID: string, acDesc: string, evidence: string) {
		const response = await fetch(`/api/tickets/${ticketID}`, {
			method: 'PATCH',
			headers: { 'content-type': 'application/json' },
			body: JSON.stringify({
				kind: 'ac-complete',
				value: acDesc,
				evidence,
				projectId: data.activeProjectId
			})
		});
		const payload = (await response.json().catch(() => ({}))) as MutationResult;
		if (!response.ok || !payload.ok) {
			return { ok: false, error: payload.error ?? 'AC update failed.' };
		}
		const url = new URL($page.url);
		if (data.activeProjectId) url.searchParams.set('project', data.activeProjectId);
		await goto(url.toString(), { invalidateAll: true });
		await refreshSecureStatus();
		return { ok: true };
	}

	async function addComment(ticketID: string, body: string) {
		const response = await fetch(`/api/tickets/${ticketID}`, {
			method: 'PATCH',
			headers: { 'content-type': 'application/json' },
			body: JSON.stringify({
				kind: 'comment',
				value: body,
				projectId: data.activeProjectId
			})
		});
		const payload = (await response.json().catch(() => ({}))) as MutationResult;
		if (!response.ok || !payload.ok) {
			return { ok: false, error: payload.error ?? 'Failed to add comment.' };
		}
		const url = new URL($page.url);
		if (data.activeProjectId) url.searchParams.set('project', data.activeProjectId);
		await goto(url.toString(), { invalidateAll: true });
		await refreshSecureStatus();
		return { ok: true };
	}
</script>

<main class="min-h-screen bg-gradient-to-br from-background via-background to-muted/30 px-4 py-5 sm:px-6">
	<div class="mx-auto flex w-full max-w-[1400px] flex-col gap-4">
		<Card class="border-border/80 bg-card/90 shadow-sm">
			<CardHeader class="flex flex-row items-start justify-between gap-3">
				<div class="space-y-1">
					<CardTitle class="text-2xl tracking-tight">Docket UI</CardTitle>
					<p class="text-sm text-muted-foreground">
						{filtered.length} of {data.tickets.length} tickets visible
					</p>
				</div>
				<div class="flex items-center gap-3">
					<div class="flex items-center gap-2">
						<span class="text-xs text-muted-foreground">Project</span>
						<select
							class="rounded-md border border-input bg-background px-2 py-1 text-sm shadow-xs focus:outline-none focus:ring-2 focus:ring-ring/40"
							value={data.activeProjectId ?? ''}
							onchange={(e) => switchProject((e.target as HTMLSelectElement).value)}
						>
							{#each data.projects as project}
								<option value={project.id}>{project.name} ({project.id})</option>
							{/each}
						</select>
						<Button variant="ghost" size="sm" class="h-8 w-8 p-0" onclick={addProject} title="Add Project">
							<span class="text-lg">+</span>
						</Button>
					</div>
					<Badge variant="secondary">{openStates.length} workflow states</Badge>
					<Button
						variant={showClosedStates ? 'default' : 'outline'}
						size="sm"
						onclick={() => {
							showClosedStates = !showClosedStates;
						}}
					>
						{showClosedStates ? 'Hide Closed' : 'Show Closed'}
					</Button>
					<Button variant="outline" size="sm" onclick={() => applyStatePreset(['in-review'])}>
						In Review
					</Button>
					<Button variant="outline" size="sm" onclick={() => applyStatePreset(['done'])}>
						Done
					</Button>
					<Button variant="outline" size="sm" onclick={() => applyStatePreset(visibleStates)}>
						All Visible
					</Button>
					<Button size="sm" onclick={() => (createModalOpen = true)}>
						Add Ticket (n)
					</Button>
				</div>
			</CardHeader>
			<CardContent>
				<FilterBar
					bind:this={filterBar}
					bind:searchQuery
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
			<TabsList class="w-fit border border-border bg-background/80 p-1 shadow-xs">
				<TabsTrigger value="board">Board</TabsTrigger>
				<TabsTrigger value="list">List</TabsTrigger>
				<TabsTrigger value="review">Review Queue</TabsTrigger>
				<TabsTrigger value="health">Health</TabsTrigger>
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

			<TabsContent value="review" class="mt-0">
				<ListTable
					tickets={reviewQueue}
					{sortBy}
					{sortDir}
					on:sort={(e: CustomEvent<{ by: SortKey }>) => toggleSort(e.detail.by)}
					on:select={(e: CustomEvent<{ ticket: Ticket }>) => onCardSelect(e.detail.ticket)}
				/>
			</TabsContent>

			<TabsContent value="health" class="mt-0">
				<HealthView projectId={data.activeProjectId} on:select={(e: CustomEvent<{ id: string }>) => {
					const t = data.tickets.find((t: Ticket) => t.id === e.detail.id);
					if (t) onCardSelect(t);
				}} />
			</TabsContent>
		</Tabs>
	</div>

	<DetailSheet
		ticket={selectedTicket}
		bind:open={sheetOpen}
		stateOptions={allStates}
		relations={data.relations}
		onUpdateState={updateState}
		onUpdateTitle={updateTitle}
		onUpdateDescription={updateDescription}
		onUpdateAC={updateAC}
		onAddComment={addComment}
		secureActive={secureActive}
		secureExpiresAt={secureExpiresAt}
		secureStatusError={secureStatusError}
		onSelect={(e) => {
			const t = data.tickets.find((t: Ticket) => t.id === e.detail.id);
			if (t) onCardSelect(t);
		}}
	/>

	<CreateTicketModal
		bind:open={createModalOpen}
		config={data.config}
		projectId={data.activeProjectId}
		onCreate={async (t: Ticket) => {
			onCardSelect(t);
		}}
	/>
</main>
