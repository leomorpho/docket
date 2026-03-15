<script lang="ts">
	import { marked } from 'marked';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import HumanDateTime from '$lib/components/HumanDateTime.svelte';
	import TicketHierarchyTree from '$lib/components/TicketHierarchyTree.svelte';
	import TicketReference from '$lib/components/TicketReference.svelte';
	import TicketTimeline from '$lib/components/TicketTimeline.svelte';
	import { ScrollArea } from '$lib/components/ui/scroll-area';
	import { Select, SelectContent, SelectItem, SelectTrigger } from '$lib/components/ui/select';
	import {
		Sheet,
		SheetContent,
		SheetDescription,
		SheetHeader,
		SheetTitle
	} from '$lib/components/ui/sheet';
	import { buildChildrenByParent } from '$lib/hierarchy';
	import type { Relation, Ticket } from '$lib/types';

	type MutationResult = { ok: boolean; error?: string };
	type StateOption = { key: string; label: string };
	type StateUpdateOptions = { approvalTicket?: string; confirmed?: boolean };
	type RelatedTicket = { id: string; title: string; score: number };
	type HierarchyNode = { ticket: Ticket; children: HierarchyNode[] };

	let {
		ticket,
		tickets,
		projectId = null,
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
		canNavigateBack = false,
		canNavigateForward = false,
		onNavigateBack,
		onNavigateForward,
		relations = [],
		onSelect
	} = $props<{
		ticket: Ticket | null;
		tickets: Ticket[];
		projectId?: string | null;
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
		canNavigateBack?: boolean;
		canNavigateForward?: boolean;
		onNavigateBack?: () => void;
		onNavigateForward?: () => void;
		onSelect?: (e: CustomEvent<{ id: string }>) => void;
	}>();

	const overviewMarkdown = $derived.by(() => {
		if (!ticket) return '_No overview provided._';
		const source = ticket.description?.trim() || extractDescription(ticket.body);
		return source || '_No overview provided._';
	});
	const overviewHtml = $derived.by(() => marked.parse(overviewMarkdown, { gfm: true, breaks: true }));
	const handoffHtml = $derived.by(() =>
		ticket?.handoff?.trim() ? marked.parse(ticket.handoff, { gfm: true, breaks: true }) : ''
	);
	const frontmatterEntries = $derived.by(() =>
		Object.entries(ticket?.frontmatter ?? {}).sort(([left], [right]) => left.localeCompare(right))
	);

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
		descriptionDraft = ticket.description ?? extractDescription(ticket.body);
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
	const ticketByID = $derived.by(
		() => new Map<string, Ticket>(tickets.map((t: Ticket): [string, Ticket] => [t.id, t]))
	);
	const childrenByParent = $derived.by(() => buildChildrenByParent(tickets));
	const childCount = $derived(ticket ? (childrenByParent.get(ticket.id)?.length ?? 0) : 0);
	const parentTrail = $derived.by((): Ticket[] => {
		if (!ticket) return [];
		const trail: Ticket[] = [];
		const seen = new Set<string>([ticket.id]);
		let parentID = ticket.parent;
		while (parentID) {
			if (seen.has(parentID)) break;
			const parent = ticketByID.get(parentID);
			if (!parent) break;
			trail.unshift(parent);
			seen.add(parent.id);
			parentID = parent.parent;
		}
		return trail;
	});
	const siblings = $derived.by((): Ticket[] => {
		if (!ticket?.parent) return [];
		return (childrenByParent.get(ticket.parent) ?? []).filter((t) => t.id !== ticket.id);
	});
	const childHierarchy = $derived.by((): HierarchyNode[] => {
		if (!ticket) return [];
		const build = (parentID: string, seen: Set<string>): HierarchyNode[] => {
			const children = childrenByParent.get(parentID) ?? [];
			const nodes: HierarchyNode[] = [];
			for (const child of children) {
				if (seen.has(child.id)) continue;
				const nextSeen = new Set(seen);
				nextSeen.add(child.id);
				nodes.push({
					ticket: child,
					children: build(child.id, nextSeen)
				});
			}
			return nodes;
		};
		return build(ticket.id, new Set([ticket.id]));
	});

	function extractDescription(markdown: string): string {
		const lines = markdown.split('\n');
		const titleLine = lines.findIndex((line) => line.startsWith('# '));
		const start = titleLine >= 0 ? titleLine + 1 : 0;
		let effectiveStart = start;
		for (let i = start; i < lines.length; i += 1) {
			const line = lines[i].trim();
			if (!line) continue;
			if (/^##\s+Description\b/i.test(line)) {
				effectiveStart = i + 1;
			}
			break;
		}
		let end = lines.findIndex(
			(line, idx) =>
				idx > effectiveStart &&
				/^\s*##\s+(Acceptance Criteria|Plan|Comments|Handoff)\b/i.test(line)
		);
		if (end < 0) end = lines.length;
		return lines.slice(effectiveStart, end).join('\n').trim();
	}

	function resetQuickEdit() {
		if (!ticket) return;
		titleDraft = ticket.title;
		descriptionDraft = ticket.description ?? extractDescription(ticket.body);
	}

	function selectTicketByID(id: string) {
		onSelect?.(new CustomEvent('select', { detail: { id } }));
	}

	function isTicketID(value: string): boolean {
		return /^TKT-\d+$/.test(value);
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
		const current = (ticket.description ?? extractDescription(ticket.body)).trim();
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
			const query = projectId ? `?projectId=${encodeURIComponent(projectId)}` : '';
			const response = await fetch(`/api/tickets/${ticket.id}/related${query}`);
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
	<SheetContent class="w-[96vw] max-w-none border-l-border bg-card/98 p-0 sm:w-[92vw] lg:w-[80vw]">
		{#if ticket}
			<div class="flex h-full flex-col">
				<SheetHeader class="space-y-3 border-b border-border/80 px-6 pt-6 pb-5">
					<div class="flex items-start justify-between gap-3">
						<div class="space-y-1">
							<SheetTitle class="text-xl leading-tight font-semibold text-foreground">{ticket.id}</SheetTitle>
							<SheetDescription class="text-sm text-foreground">{ticket.title}</SheetDescription>
						</div>
						<div class="flex items-center gap-2">
							<Button
								variant="outline"
								size="sm"
								onclick={() => onNavigateBack?.()}
								disabled={!canNavigateBack}
								aria-label="Previous ticket"
								title="Previous ticket (Alt+Left)"
							>
								Back
							</Button>
							<Button
								variant="outline"
								size="sm"
								onclick={() => onNavigateForward?.()}
								disabled={!canNavigateForward}
								aria-label="Next ticket"
								title="Next ticket (Alt+Right)"
							>
								Forward
							</Button>
							<Button variant="outline" size="sm" onclick={() => (open = false)}>Close</Button>
						</div>
					</div>

					<div class="flex flex-wrap items-center gap-2">
						<Badge variant="outline">{ticket.state}</Badge>
						<Badge variant="secondary">P{ticket.priority}</Badge>
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
							{#if secureActive && secureExpiresAt}
								<span class="ml-1">(expires <HumanDateTime value={secureExpiresAt} layout="inline" />)</span>
							{/if}
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
							<article class="overview-markdown max-w-none rounded-lg border border-border bg-background p-4 text-sm leading-relaxed text-foreground">
								{@html overviewHtml}
							</article>
						</section>

						<section class="space-y-3">
							<p class="text-xs font-medium tracking-wide text-muted-foreground uppercase">Timeline</p>
							<div class="rounded-lg border border-border bg-muted/30 p-4">
								<TicketTimeline {ticket} />
							</div>
						</section>

						<section class="space-y-3">
							<p class="text-xs font-medium tracking-wide text-muted-foreground uppercase">Hierarchy</p>
							<div class="space-y-4 rounded-lg border border-border bg-muted/30 p-4">
								{#if parentTrail.length > 0}
									<div>
										<p class="text-[11px] font-semibold tracking-wide text-muted-foreground uppercase">Parent Path</p>
										<div class="mt-2 flex flex-wrap items-center gap-2">
											{#each parentTrail as ancestor}
												<TicketReference
													id={ancestor.id}
													ticket={ancestor}
													on:select={(e: CustomEvent<{ id: string }>) => selectTicketByID(e.detail.id)}
												/>
											{/each}
											<Badge variant="secondary" class="font-mono text-[10px]">{ticket.id}</Badge>
										</div>
									</div>
								{/if}

								{#if siblings.length > 0}
									<div>
										<p class="text-[11px] font-semibold tracking-wide text-muted-foreground uppercase">
											Siblings
										</p>
										<div class="mt-2 flex flex-wrap gap-2">
											{#each siblings as sibling}
												<TicketReference
													id={sibling.id}
													ticket={sibling}
													on:select={(e: CustomEvent<{ id: string }>) => selectTicketByID(e.detail.id)}
												/>
											{/each}
										</div>
									</div>
								{/if}

								<div>
									<p class="text-[11px] font-semibold tracking-wide text-muted-foreground uppercase">Children</p>
									{#if childHierarchy.length === 0}
										<p class="mt-2 text-xs text-muted-foreground italic">No child tickets.</p>
									{:else}
										<div class="mt-2">
											<TicketHierarchyTree
												nodes={childHierarchy}
												on:select={(e: CustomEvent<{ id: string }>) => selectTicketByID(e.detail.id)}
											/>
										</div>
									{/if}
								</div>
							</div>
						</section>

						<section class="space-y-3">
							<p class="text-xs font-medium tracking-wide text-muted-foreground uppercase">Metadata</p>
							<div class="space-y-3 rounded-lg border border-border bg-muted/30 p-4">
								<div class="grid gap-2 sm:grid-cols-2">
									<div class="rounded-md border border-border bg-background px-3 py-2 text-xs">
										<p class="text-muted-foreground">Created By</p>
										<p class="mt-1 font-mono text-foreground">{ticket.created_by ?? '—'}</p>
									</div>
									<div class="rounded-md border border-border bg-background px-3 py-2 text-xs">
										<p class="text-muted-foreground">Write Hash</p>
										<p class="mt-1 break-all font-mono text-foreground">{ticket.write_hash ?? '—'}</p>
									</div>
								</div>

								{#if (ticket.blocked_by?.length ?? 0) > 0}
									<div>
										<p class="text-[11px] font-semibold tracking-wide text-muted-foreground uppercase">
											Blocked By
										</p>
										<div class="mt-2 flex flex-wrap gap-2">
											{#each ticket.blocked_by ?? [] as blocker}
												{#if isTicketID(blocker)}
													<TicketReference
														id={blocker}
														ticket={ticketByID.get(blocker)}
														on:select={(e: CustomEvent<{ id: string }>) => selectTicketByID(e.detail.id)}
													/>
												{:else}
													<Badge variant="outline" class="font-mono text-[10px]">{blocker}</Badge>
												{/if}
											{/each}
										</div>
									</div>
								{/if}

								{#if frontmatterEntries.length > 0}
									<div class="rounded-md border border-border bg-background">
										<div class="border-b border-border px-3 py-2 text-[11px] font-semibold tracking-wide text-muted-foreground uppercase">
											Frontmatter
										</div>
										<div class="divide-y divide-border">
											{#each frontmatterEntries as [frontmatterKey, frontmatterValue]}
												<div class="grid gap-2 px-3 py-2 text-xs sm:grid-cols-[10rem_1fr]">
													<p class="font-mono text-muted-foreground">{frontmatterKey}</p>
													{#if Array.isArray(frontmatterValue)}
														<div class="flex flex-wrap gap-1.5">
															{#each frontmatterValue as item}
																{#if isTicketID(item)}
																	<TicketReference
																		id={item}
																		ticket={ticketByID.get(item)}
																		compact={true}
																		on:select={(e: CustomEvent<{ id: string }>) =>
																			selectTicketByID(e.detail.id)}
																	/>
																{:else}
																	<Badge variant="outline" class="font-mono text-[10px]">{item}</Badge>
																{/if}
															{/each}
														</div>
													{:else}
														<p class="break-all font-mono text-foreground">{frontmatterValue}</p>
													{/if}
												</div>
											{/each}
										</div>
									</div>
								{/if}
							</div>
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

						{#if ticket.handoff}
							<section class="space-y-3">
								<p class="text-xs font-medium tracking-wide text-muted-foreground uppercase">Handoff</p>
								<article class="overview-markdown max-w-none rounded-lg border border-border bg-background p-4 text-sm leading-relaxed text-foreground">
									{@html handoffHtml}
								</article>
							</section>
						{/if}

						<section class="space-y-4 border-t border-border pt-6">
							<p class="text-xs font-medium tracking-wide text-muted-foreground uppercase">Comments</p>
							<div class="space-y-4">
								{#if (ticket.comments?.length ?? 0) === 0}
									<p class="text-xs text-muted-foreground italic">No comments recorded yet.</p>
								{/if}
								{#each ticket.comments || [] as comment}
									<div class="rounded-lg border border-border bg-background p-3 shadow-xs">
										<div class="mb-2 flex items-center justify-between gap-2">
											<span class="text-xs font-semibold text-foreground">{comment.author}</span>
											<HumanDateTime value={comment.at} layout="inline" />
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
								Raw Markdown
							</summary>
							<div class="mt-3 space-y-2">
								<p class="text-[11px] text-muted-foreground">
									Full ticket body exactly as stored in markdown.
								</p>
								<pre class="max-h-72 overflow-auto rounded-md border border-border bg-background p-3 text-[11px] leading-relaxed text-foreground">{ticket.body}</pre>
							</div>
						</details>

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
											<TicketReference
												id={rel.to}
												ticket={ticketByID.get(rel.to)}
												on:select={(e: CustomEvent<{ id: string }>) =>
													onSelect?.(new CustomEvent('select', { detail: { id: e.detail.id } }))}
											/>
										{:else}
											<span class="text-xs text-muted-foreground italic">None</span>
										{/each}
									</div>
								</div>
								<div class="rounded-lg border border-border bg-background p-3">
									<p class="mb-2 text-xs font-semibold text-muted-foreground">Dependents</p>
									<div class="flex flex-wrap gap-2">
										{#each relations.filter((r: Relation) => r.to === ticket.id && r.relation === 'blocked_by') as rel}
											<TicketReference
												id={rel.from}
												ticket={ticketByID.get(rel.from)}
												on:select={(e: CustomEvent<{ id: string }>) =>
													onSelect?.(new CustomEvent('select', { detail: { id: e.detail.id } }))}
											/>
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

	.overview-markdown :global(h1),
	.overview-markdown :global(h2),
	.overview-markdown :global(h3) {
		margin-top: 1rem;
		margin-bottom: 0.5rem;
		font-weight: 700;
		line-height: 1.2;
	}

	.overview-markdown :global(h1) {
		font-size: 1.1rem;
	}

	.overview-markdown :global(h2) {
		font-size: 1rem;
	}

	.overview-markdown :global(h3) {
		font-size: 0.95rem;
	}

	.overview-markdown :global(p) {
		margin-top: 0.65rem;
	}

	.overview-markdown :global(ul),
	.overview-markdown :global(ol) {
		margin-top: 0.65rem;
		padding-left: 1.1rem;
	}

	.overview-markdown :global(li) {
		margin-top: 0.35rem;
	}

	.overview-markdown :global(blockquote) {
		margin-top: 0.8rem;
		border-left: 3px solid hsl(var(--border));
		padding-left: 0.75rem;
		color: hsl(var(--muted-foreground));
	}

	.overview-markdown :global(a) {
		color: hsl(var(--primary));
		text-decoration: underline;
		text-underline-offset: 2px;
	}

	.overview-markdown :global(hr) {
		margin-top: 1rem;
		border: 0;
		border-top: 1px solid hsl(var(--border));
	}

	.overview-markdown :global(table) {
		margin-top: 0.8rem;
		width: 100%;
		border-collapse: collapse;
	}

	.overview-markdown :global(th),
	.overview-markdown :global(td) {
		border: 1px solid hsl(var(--border));
		padding: 0.4rem 0.5rem;
		text-align: left;
	}

	.overview-markdown :global(pre) {
		margin-top: 0.8rem;
		border-radius: 0.5rem;
		background: #0f172a;
		color: #f1f5f9;
		padding: 1rem;
		overflow: auto;
	}

	.overview-markdown :global(code) {
		font-family: 'JetBrains Mono', ui-monospace, SFMono-Regular, Menlo, monospace;
	}
</style>
