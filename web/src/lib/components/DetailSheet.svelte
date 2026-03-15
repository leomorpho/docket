<script lang="ts">
	import { marked } from 'marked';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { ScrollArea } from '$lib/components/ui/scroll-area';
	import { Select, SelectContent, SelectItem, SelectTrigger } from '$lib/components/ui/select';
	import {
		Sheet,
		SheetContent,
		SheetDescription,
		SheetHeader,
		SheetTitle
	} from '$lib/components/ui/sheet';
	import type { Relation, Ticket } from '$lib/types';

	type MutationResult = { ok: boolean; error?: string };
	type StateOption = { key: string; label: string };
	type StateUpdateOptions = { approvalTicket?: string; confirmed?: boolean };
	type RelatedTicket = { id: string; title: string; score: number };

	let {
		ticket,
		open = $bindable(false),
		stateOptions,
		onUpdateState,
		onUpdateTitle,
		onUpdateDescription,
		onUpdateAC,
		onAddComment,
		secureActive = false,
		secureExpiresAt = '',
		secureStatusError = '',
		relations = [],
		onSelect
	} = $props<{
		ticket: Ticket | null;
		open: boolean;
		stateOptions: StateOption[];
		relations: Relation[];
		onUpdateState: (ticketID: string, state: string, options?: StateUpdateOptions) => Promise<MutationResult>;
		onUpdateTitle: (ticketID: string, title: string) => Promise<MutationResult>;
		onUpdateDescription: (ticketID: string, description: string) => Promise<MutationResult>;
		onUpdateAC: (ticketID: string, acDesc: string, evidence: string) => Promise<MutationResult>;
		onAddComment: (ticketID: string, body: string) => Promise<MutationResult>;
		secureActive?: boolean;
		secureExpiresAt?: string;
		secureStatusError?: string;
		onSelect?: (e: CustomEvent<{ id: string }>) => void;
	}>();

	const html = $derived(ticket ? marked.parse(ticket.body, { gfm: true }) : '');

	let stateDraft = $state('');
	let titleDraft = $state('');
	let descriptionDraft = $state('');
	let quickEditOpen = $state(false);
	let savingState = $state(false);
	let savingTitle = $state(false);
	let savingDescription = $state(false);
	let errorMessage = $state('');
	let successMessage = $state('');
	let approvalTicket = $state('');
	let confirmPrivileged = $state(false);
	let newCommentBody = $state('');
	let savingComment = $state(false);
	let relatedTickets = $state<RelatedTicket[]>([]);
	let loadingRelated = $state(false);

	$effect(() => {
		if (!ticket) {
			stateDraft = '';
			titleDraft = '';
			descriptionDraft = '';
			quickEditOpen = false;
			return;
		}
		stateDraft = ticket.state;
		titleDraft = ticket.title;
		descriptionDraft = extractDescription(ticket.body);
		approvalTicket = ticket.id;
		confirmPrivileged = false;
		errorMessage = '';
		successMessage = '';
		quickEditOpen = false;
	});

	$effect(() => {
		if (ticket && open) {
			fetchRelated();
		} else {
			relatedTickets = [];
		}
	});

	const stateIsPrivileged = $derived(stateDraft === 'done' || stateDraft === 'archived');

	function extractDescription(markdown: string): string {
		const lines = markdown.split('\n');
		const titleLine = lines.findIndex((line) => line.startsWith('# '));
		const start = titleLine >= 0 ? titleLine + 1 : 0;
		let end = lines.findIndex((line, idx) => idx > start && line.startsWith('## Acceptance Criteria'));
		if (end < 0) end = lines.length;
		return lines.slice(start, end).join('\n').trim();
	}

	function resetQuickEdit() {
		if (!ticket) return;
		titleDraft = ticket.title;
		descriptionDraft = extractDescription(ticket.body);
	}

	async function saveState() {
		if (!ticket || !stateDraft || stateDraft === ticket.state || savingState) return;
		if (stateIsPrivileged) {
			if (!confirmPrivileged) {
				errorMessage = 'Privileged transition requires explicit confirmation.';
				successMessage = '';
				return;
			}
			if (!approvalTicket.trim()) {
				errorMessage = 'Approval ticket is required for privileged transitions.';
				successMessage = '';
				return;
			}
		}
		savingState = true;
		errorMessage = '';
		successMessage = '';
		const result = await onUpdateState(
			ticket.id,
			stateDraft,
			stateIsPrivileged
				? { approvalTicket: approvalTicket.trim(), confirmed: confirmPrivileged }
				: undefined
		);
		savingState = false;
		if (!result.ok) {
			errorMessage = result.error ?? 'Failed to update state.';
			return;
		}
		successMessage = 'State updated.';
	}

	async function saveTitle() {
		if (!ticket || savingTitle) return;
		const next = titleDraft.trim();
		if (!next) {
			errorMessage = 'Title cannot be empty.';
			successMessage = '';
			return;
		}
		if (next === ticket.title) return;
		savingTitle = true;
		errorMessage = '';
		successMessage = '';
		const result = await onUpdateTitle(ticket.id, next);
		savingTitle = false;
		if (!result.ok) {
			errorMessage = result.error ?? 'Failed to update title.';
			return;
		}
		successMessage = 'Title updated.';
	}

	async function saveDescription() {
		if (!ticket || savingDescription) return;
		const next = descriptionDraft.trim();
		const current = extractDescription(ticket.body);
		if (next === current) return;
		savingDescription = true;
		errorMessage = '';
		successMessage = '';
		const result = await onUpdateDescription(ticket.id, next);
		savingDescription = false;
		if (!result.ok) {
			errorMessage = result.error ?? 'Failed to update description.';
			return;
		}
		successMessage = 'Description updated.';
	}

	async function fetchRelated() {
		if (!ticket) return;
		loadingRelated = true;
		try {
			const response = await fetch(`/api/tickets/${ticket.id}/related`);
			const data = await response.json();
			if (data.ok) {
				relatedTickets = data.related || [];
			}
		} catch {
			relatedTickets = [];
		} finally {
			loadingRelated = false;
		}
	}

	async function addComment() {
		if (!ticket || !newCommentBody.trim() || savingComment) return;
		savingComment = true;
		const result = await onAddComment(ticket.id, newCommentBody.trim());
		savingComment = false;
		if (result.ok) {
			newCommentBody = '';
			successMessage = 'Comment added.';
			errorMessage = '';
		} else {
			errorMessage = result.error ?? 'Failed to add comment.';
		}
	}
</script>

<Sheet bind:open>
	<SheetContent class="w-[min(64rem,96vw)] max-w-none border-l-border bg-card/98 p-0 sm:w-[min(56rem,92vw)]">
		{#if ticket}
			<div class="flex h-full flex-col">
				<SheetHeader class="space-y-3 border-b border-border/80 px-6 pt-6 pb-5">
					<div class="flex items-start justify-between gap-3">
						<div class="space-y-1">
							<SheetTitle class="text-xl leading-tight font-semibold text-foreground">{ticket.id}</SheetTitle>
							<SheetDescription class="text-sm text-foreground">{ticket.title}</SheetDescription>
						</div>
						<Button variant="outline" size="sm" onclick={() => (open = false)}>Close</Button>
					</div>

					<div class="flex flex-wrap items-center gap-2">
						<Badge variant="outline">{ticket.state}</Badge>
						<Badge variant="secondary">P{ticket.priority}</Badge>
						<Badge variant="outline">created {ticket.created_at}</Badge>
						<Badge variant="outline">updated {ticket.updated_at}</Badge>
						{#if ticket.parent}<Badge variant="outline">parent: {ticket.parent}</Badge>{/if}
						{#each ticket.labels as label}
							<Badge variant="secondary" class="bg-muted text-foreground">{label}</Badge>
						{/each}
					</div>

					<div class="flex flex-wrap items-center gap-2">
						<Select type="single" value={stateDraft} onValueChange={(value: string) => (stateDraft = value)}>
							<SelectTrigger class="w-52 bg-background">
								{stateOptions.find((s: StateOption) => s.key === stateDraft)?.label ?? stateDraft}
							</SelectTrigger>
							<SelectContent>
								{#each stateOptions as state}
									<SelectItem value={state.key}>{state.label}</SelectItem>
								{/each}
							</SelectContent>
						</Select>
						<Button size="sm" variant="secondary" onclick={saveState} disabled={savingState}>
							{savingState ? 'Updating...' : stateIsPrivileged ? 'Request privileged update' : 'Update state'}
						</Button>
						<Button size="sm" variant="outline" onclick={() => (quickEditOpen = !quickEditOpen)}>
							{quickEditOpen ? 'Hide quick edit' : 'Quick edit'}
						</Button>
					</div>

					{#if stateIsPrivileged}
						<div class="rounded-md border border-amber-300 bg-amber-50 p-3 text-sm">
							<p class="font-medium text-amber-900">Privileged transition required</p>
							<p class="mt-1 text-amber-800">
								This change must run through secure mode with explicit approval.
							</p>
							<div class="mt-2 flex flex-col gap-2 sm:flex-row sm:items-center">
								<input
									class="h-9 rounded-md border border-amber-300/70 bg-background px-3 text-sm"
									value={approvalTicket}
									oninput={(e) => (approvalTicket = (e.currentTarget as HTMLInputElement).value)}
									placeholder="Approval ticket (TKT-###)"
								/>
								<label class="flex items-center gap-2 text-xs font-medium text-amber-900">
									<input
										type="checkbox"
										checked={confirmPrivileged}
										onchange={(e) => (confirmPrivileged = (e.currentTarget as HTMLInputElement).checked)}
									/>
									I explicitly approve this privileged transition.
								</label>
							</div>
						</div>
					{/if}

					<div class="rounded-md border border-border bg-muted/40 px-3 py-2 text-xs text-foreground">
						<p>
							Secure mode: <span class={secureActive ? 'text-emerald-700' : 'text-red-700'}>{secureActive ? 'active' : 'inactive'}</span>
							{#if secureActive && secureExpiresAt} (expires: {secureExpiresAt}){/if}
						</p>
						{#if secureStatusError}
							<p class="mt-1 text-red-700">{secureStatusError}</p>
						{/if}
					</div>

					{#if errorMessage}
						<p class="text-xs text-red-600">{errorMessage}</p>
					{/if}
					{#if successMessage}
						<p class="text-xs text-emerald-600">{successMessage}</p>
					{/if}
				</SheetHeader>

				<ScrollArea class="h-[calc(100vh-14.5rem)] px-6 py-5">
					<div class="space-y-6 pb-6">
						<section class="space-y-3">
							<p class="text-xs font-medium tracking-wide text-muted-foreground uppercase">Overview</p>
							<article class="markdown max-w-none rounded-lg border border-border bg-background p-4 text-sm leading-relaxed text-foreground">
								{@html html}
							</article>
						</section>

						{#if quickEditOpen}
							<section class="space-y-3 rounded-lg border border-border bg-muted/40 p-4">
								<p class="text-xs font-medium tracking-wide text-muted-foreground uppercase">Quick Edit</p>
								<div class="grid gap-2 sm:grid-cols-[1fr_auto]">
									<input
										class="h-9 rounded-md border border-input bg-background px-3 text-sm"
										value={titleDraft}
										oninput={(e) => (titleDraft = (e.currentTarget as HTMLInputElement).value)}
										placeholder="Ticket title"
										disabled={savingTitle}
									/>
									<Button size="sm" onclick={saveTitle} disabled={savingTitle}>
										{savingTitle ? 'Saving...' : 'Save title'}
									</Button>
								</div>
								<div class="grid gap-2">
									<textarea
										class="min-h-36 w-full rounded-md border border-input bg-background p-3 text-sm"
										value={descriptionDraft}
										oninput={(e) => (descriptionDraft = (e.currentTarget as HTMLTextAreaElement).value)}
										disabled={savingDescription}
									></textarea>
									<div class="flex flex-wrap gap-2">
										<Button size="sm" onclick={saveDescription} disabled={savingDescription}>
											{savingDescription ? 'Saving...' : 'Save description'}
										</Button>
										<Button size="sm" variant="outline" onclick={resetQuickEdit} disabled={savingDescription}>
											Reset
										</Button>
									</div>
								</div>
							</section>
						{/if}

						{#if ticket.ac.length > 0}
							<section class="space-y-3">
								<p class="text-xs font-medium tracking-wide text-muted-foreground uppercase">
									Acceptance Criteria
								</p>
								<div class="space-y-2">
									{#each ticket.ac as ac}
										<div class="flex items-start gap-3 rounded-md border border-border bg-background p-3">
											<input
												type="checkbox"
												checked={ac.done}
												class="mt-1 h-4 w-4 rounded border-input text-primary"
												onchange={async (e) => {
													if ((e.target as HTMLInputElement).checked) {
														const evidence = prompt(
															'Provide evidence for completion (e.g. commit SHA, file path, or description):'
														);
														if (evidence !== null) {
															await onUpdateAC(ticket.id, ac.description, evidence);
														} else {
															(e.target as HTMLInputElement).checked = false;
														}
													}
												}}
												disabled={ac.done}
											/>
											<div class="flex-1">
												<p class="text-sm {ac.done ? 'text-muted-foreground line-through' : 'text-foreground'}">
													{ac.description}
												</p>
												{#if ac.evidence}
													<p class="mt-1 text-xs text-emerald-600">Evidence: {ac.evidence}</p>
												{/if}
											</div>
										</div>
									{/each}
								</div>
							</section>
						{/if}

						<section class="space-y-4 border-t border-border pt-6">
							<p class="text-xs font-medium tracking-wide text-muted-foreground uppercase">Comments</p>
							<div class="space-y-4">
								{#each ticket.comments || [] as comment}
									<div class="rounded-lg border border-border bg-background p-3 shadow-xs">
										<div class="mb-2 flex items-center justify-between gap-2">
											<span class="text-xs font-semibold text-foreground">{comment.author}</span>
											<span class="text-[10px] text-muted-foreground">{new Date(comment.at).toLocaleString()}</span>
										</div>
										<div class="markdown text-sm text-foreground">
											{@html marked.parse(comment.body, { gfm: true })}
										</div>
									</div>
								{/each}

								<div class="space-y-2">
									<textarea
										class="min-h-24 w-full rounded-md border border-input bg-muted/30 p-3 text-sm focus:bg-background focus:outline-none focus:ring-2 focus:ring-ring/40"
										placeholder="Add a comment... (Markdown supported)"
										bind:value={newCommentBody}
									></textarea>
									<Button size="sm" onclick={addComment} disabled={savingComment || !newCommentBody.trim()}>
										{savingComment ? 'Adding...' : 'Add comment'}
									</Button>
								</div>
							</div>
						</section>

						<details class="rounded-lg border border-border bg-muted/30 p-4">
							<summary class="cursor-pointer text-xs font-medium tracking-wide text-muted-foreground uppercase">
								Related Tickets
							</summary>
							<div class="mt-3">
								{#if loadingRelated}
									<p class="text-xs text-muted-foreground animate-pulse">Finding similar tickets...</p>
								{:else if relatedTickets.length === 0}
									<p class="text-xs text-muted-foreground italic">No similar tickets found.</p>
								{:else}
									<div class="space-y-2">
										{#each relatedTickets as rel}
											<button
												class="w-full rounded-lg border border-border bg-background p-3 text-left shadow-xs transition-colors hover:border-primary/50 hover:bg-accent/30"
												onclick={() => onSelect?.(new CustomEvent('select', { detail: { id: rel.id } }))}
											>
												<div class="flex items-center justify-between gap-2">
													<span class="text-xs font-mono font-bold text-muted-foreground">{rel.id}</span>
													<Badge variant="outline" class="text-[10px]">Score: {rel.score.toFixed(2)}</Badge>
												</div>
												<p class="mt-1 text-sm font-medium text-foreground line-clamp-1">{rel.title}</p>
											</button>
										{/each}
									</div>
								{/if}
							</div>
						</details>

						<details class="rounded-lg border border-border bg-muted/30 p-4">
							<summary class="cursor-pointer text-xs font-medium tracking-wide text-muted-foreground uppercase">
								Relations
							</summary>
							<div class="mt-3 grid gap-4 sm:grid-cols-2">
								<div class="rounded-lg border border-border bg-background p-3">
									<p class="mb-2 text-xs font-semibold text-muted-foreground">Blockers</p>
									<div class="flex flex-wrap gap-2">
										{#each relations.filter((r: Relation) => r.from === ticket.id && r.relation === 'blocked_by') as rel}
											<Button
												variant="outline"
												size="sm"
												class="h-7 px-2 font-mono text-xs"
												onclick={() => onSelect?.(new CustomEvent('select', { detail: { id: rel.to } }))}
											>
												{rel.to}
											</Button>
										{:else}
											<span class="text-xs text-muted-foreground italic">None</span>
										{/each}
									</div>
								</div>
								<div class="rounded-lg border border-border bg-background p-3">
									<p class="mb-2 text-xs font-semibold text-muted-foreground">Dependents</p>
									<div class="flex flex-wrap gap-2">
										{#each relations.filter((r: Relation) => r.to === ticket.id && r.relation === 'blocked_by') as rel}
											<Button
												variant="outline"
												size="sm"
												class="h-7 px-2 font-mono text-xs"
												onclick={() => onSelect?.(new CustomEvent('select', { detail: { id: rel.from } }))}
											>
												{rel.from}
											</Button>
										{:else}
											<span class="text-xs text-muted-foreground italic">None</span>
										{/each}
									</div>
								</div>
							</div>
						</details>
					</div>
				</ScrollArea>
			</div>
		{/if}
	</SheetContent>
</Sheet>

<style>
	.markdown :global(pre) {
		border-radius: 0.5rem;
		background: #0f172a;
		color: #f1f5f9;
		padding: 1rem;
		overflow: auto;
	}

	.markdown :global(code) {
		font-family: 'JetBrains Mono', ui-monospace, SFMono-Regular, Menlo, monospace;
	}
</style>
