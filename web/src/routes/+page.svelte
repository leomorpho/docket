<script lang="ts">
	import { goto, replaceState } from '$app/navigation';
	import { page } from '$app/stores';
	import BoardView from '$lib/components/BoardView.svelte';
	import CreateTicketModal from '$lib/components/CreateTicketModal.svelte';
	import DetailSheet from '$lib/components/DetailSheet.svelte';
	import FilterBar from '$lib/components/FilterBar.svelte';
	import ListTable from '$lib/components/ListTable.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Card, CardContent, CardHeader, CardTitle } from '$lib/components/ui/card';
	import { Tabs, TabsContent, TabsList, TabsTrigger } from '$lib/components/ui/tabs';
	import type { Config, Project, Relation, StateConfig, Ticket } from '$lib/types';
	import { onMount } from 'svelte';

	let { data } = $props<{
		data: {
			projects: Project[];
			activeProjectId: string | null;
			config: Config;
			tickets: Ticket[];
			relations: Relation[];
		};
	}>();

	const allStateEntries = $derived.by(() =>
		(Object.entries(data.config.states) as [string, StateConfig][])
			.sort((a, b) => a[1].column - b[1].column)
	);
	const allStates = $derived.by(() =>
		(Object.entries(data.config.states) as [string, StateConfig][])
			.sort((a, b) => a[1].column - b[1].column)
			.map(([key, st]) => ({ key, label: st.label }))
	);

	type SortKey = 'id' | 'title' | 'state' | 'priority' | 'parent' | 'created_at';
	type TicketFetchResult = { ok: boolean; ticket?: Ticket; error?: string };
	type MutationResult = { ok: boolean; error?: string };
	type StateUpdateOptions = { approvalTicket?: string; confirmed?: boolean };
	type PreloadWorkerRequest =
		| { type: 'preload'; projectId: string | null }
		| { type: 'get-ticket'; projectId: string | null; id: string };
	type PreloadWorkerResponse =
		| { type: 'ready'; projectId: string | null; count: number }
		| { type: 'ticket'; projectId: string | null; id: string; ticket: Ticket | null }
		| { type: 'error'; projectId: string | null; error: string };
	let mode = $state<'board' | 'review'>('board');
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
	let ticketCache = $state(new Map<string, Ticket>());
	let preloadWorker = $state<Worker | null>(null);

	onMount(() => {
		const handleKeydown = (e: KeyboardEvent) => {
			if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) {
				if (e.key === 'Escape') {
					(e.target as HTMLElement).blur();
				}
				return;
			}

			if (e.key === 'b') mode = 'board';
			if (e.key === 'l' || e.key === 'r') mode = 'review';
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

	onMount(() => {
		if (typeof Worker === 'undefined') {
			return;
		}
		const worker = new Worker(new URL('../lib/workers/ticket-preload.worker.ts', import.meta.url), {
			type: 'module'
		});
		preloadWorker = worker;
		worker.onmessage = (event: MessageEvent<PreloadWorkerResponse>) => {
			const msg = event.data;
			if (msg.type === 'ticket' && msg.projectId === data.activeProjectId && msg.ticket) {
				const next = new Map(ticketCache);
				next.set(msg.id, msg.ticket);
				ticketCache = next;
				if (selectedTicket?.id === msg.id) {
					selectedTicket = msg.ticket;
				}
			}
		};
		return () => {
			worker.terminate();
			preloadWorker = null;
		};
	});

	$effect(() => {
		if (data.activeProjectId) {
			localStorage.setItem('docket_active_project', data.activeProjectId);
		}
	});

	$effect(() => {
		const next = new Map<string, Ticket>();
		for (const ticket of data.tickets) {
			next.set(ticket.id, ticket);
		}
		ticketCache = next;
	});

	$effect(() => {
		void refreshSecureStatus();
	});

	$effect(() => {
		if (!preloadWorker) return;
		const request: PreloadWorkerRequest = {
			type: 'preload',
			projectId: data.activeProjectId
		};
		preloadWorker.postMessage(request);
	});

	// Handle initial ticket from URL
	$effect(() => {
		const ticketId = $page.url.searchParams.get('ticket');
		if (ticketId && !selectedTicket) {
			const t = ticketCache.get(ticketId) ?? data.tickets.find((ticket: Ticket) => ticket.id === ticketId);
			if (t) {
				selectedTicket = t;
				sheetOpen = true;
				requestWorkerTicket(ticketId);
				void hydrateTicket(ticketId);
			}
		}
	});

	$effect(() => {
		if (!sheetOpen && $page.url.searchParams.has('ticket')) {
			const url = new URL($page.url);
			url.searchParams.delete('ticket');
			replaceState(url, $page.state);
		}
	});

	// Reset view state when project changes
	$effect(() => {
		if (data.activeProjectId) {
			searchQuery = '';
			selectedTicket = null;
			sheetOpen = false;
		}
	});

	const filtered = $derived(
		data.tickets.filter((t: Ticket) => {
			if (searchQuery) {
				const q = searchQuery.toLowerCase();
				return (
					t.id.toLowerCase().includes(q) ||
					t.title.toLowerCase().includes(q)
				);
			}
			return true;
		})
	);

	const columns = $derived(allStateEntries.map(([key, st]) => ({
		key,
		label: st.label,
		column: st.column,
		tickets: filtered.filter((t: Ticket) => t.state === key).sort((a: Ticket, b: Ticket) => a.priority - b.priority || a.seq - b.seq)
	})));

	const reviewStateKeys = $derived.by(() => {
		const fromConfig = allStateEntries
			.filter(([key, st]) => key.toLowerCase().includes('review') || st.label.toLowerCase().includes('review'))
			.map(([key]) => key);
		return fromConfig.length > 0 ? fromConfig : ['in-review'];
	});
	const reviewList = $derived(
		[...filtered]
			.filter((t: Ticket) => reviewStateKeys.includes(t.state))
			.sort((a: Ticket, b: Ticket) => {
				const av = a[sortBy] ?? '';
				const bv = b[sortBy] ?? '';
				if (av < bv) return sortDir === 'asc' ? -1 : 1;
				if (av > bv) return sortDir === 'asc' ? 1 : -1;
				return 0;
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
		selectedTicket = ticketCache.get(ticket.id) ?? ticket;
		sheetOpen = true;
		const url = new URL($page.url);
		url.searchParams.set('ticket', ticket.id);
		replaceState(url, $page.state);
		requestWorkerTicket(ticket.id);
		void hydrateTicket(ticket.id);
	}

	function requestWorkerTicket(ticketID: string) {
		if (!preloadWorker) return;
		const request: PreloadWorkerRequest = {
			type: 'get-ticket',
			projectId: data.activeProjectId,
			id: ticketID
		};
		preloadWorker.postMessage(request);
	}

	async function hydrateTicket(ticketID: string) {
		const query = data.activeProjectId ? `?projectId=${encodeURIComponent(data.activeProjectId)}` : '';
		try {
			const response = await fetch(`/api/tickets/${ticketID}${query}`);
			const payload = (await response.json().catch(() => ({}))) as TicketFetchResult;
			if (!response.ok || !payload.ok || !payload.ticket) return;
			const next = new Map(ticketCache);
			next.set(ticketID, payload.ticket);
			ticketCache = next;
			if (selectedTicket?.id === ticketID) {
				selectedTicket = payload.ticket;
			}
		} catch {
			// Keep cached data on hydration failures.
		}
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
					<CardTitle class="text-2xl tracking-tight">Docket</CardTitle>
					<p class="text-sm text-muted-foreground">
						{filtered.length} of {data.tickets.length} tickets visible
					</p>
				</div>
				<div class="flex flex-wrap items-center justify-end gap-2">
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
						<Button variant="outline" size="sm" class="h-8 w-8 p-0" onclick={addProject} title="Add Project">
							<span class="text-lg">+</span>
						</Button>
					</div>
					<Button size="sm" onclick={() => (createModalOpen = true)}>
						Add Ticket (n)
					</Button>
					<Badge variant="secondary">{reviewList.length} ready for review</Badge>
				</div>
			</CardHeader>
			<CardContent>
				<FilterBar bind:this={filterBar} bind:searchQuery />
			</CardContent>
		</Card>

		<Tabs bind:value={mode} class="gap-3">
			<TabsList class="w-fit border border-border bg-background/80 p-1 shadow-xs">
				<TabsTrigger value="board">Kanban</TabsTrigger>
				<TabsTrigger value="review">Ready for Review</TabsTrigger>
			</TabsList>

			<TabsContent value="board" class="mt-0">
				<BoardView
					{columns}
					on:select={(e: CustomEvent<{ ticket: Ticket }>) => onCardSelect(e.detail.ticket)}
				/>
			</TabsContent>

			<TabsContent value="review" class="mt-0">
				<ListTable
					tickets={reviewList}
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
