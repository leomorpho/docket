<script lang="ts">
	import { marked } from 'marked';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { ScrollArea } from '$lib/components/ui/scroll-area';
	import {
		Select,
		SelectContent,
		SelectItem,
		SelectTrigger
	} from '$lib/components/ui/select';
	import {
		Sheet,
		SheetContent,
		SheetDescription,
		SheetHeader,
		SheetTitle
	} from '$lib/components/ui/sheet';
	import type { Ticket } from '$lib/types';

	type MutationResult = { ok: boolean; error?: string };
	type StateOption = { key: string; label: string };

	let {
		ticket,
		open = $bindable(false),
		stateOptions,
		onUpdateState,
		onUpdateTitle,
		onUpdateDescription
	} = $props<{
		ticket: Ticket | null;
		open: boolean;
		stateOptions: StateOption[];
		onUpdateState: (ticketID: string, state: string) => Promise<MutationResult>;
		onUpdateTitle: (ticketID: string, title: string) => Promise<MutationResult>;
		onUpdateDescription: (ticketID: string, description: string) => Promise<MutationResult>;
	}>();

	const html = $derived(ticket ? marked.parse(ticket.body, { gfm: true }) : '');

	let stateDraft = $state('');
	let titleDraft = $state('');
	let descriptionDraft = $state('');
	let savingState = $state(false);
	let savingTitle = $state(false);
	let savingDescription = $state(false);
	let errorMessage = $state('');
	let successMessage = $state('');

	$effect(() => {
		if (!ticket) {
			stateDraft = '';
			titleDraft = '';
			descriptionDraft = '';
			return;
		}
		stateDraft = ticket.state;
		titleDraft = ticket.title;
		descriptionDraft = extractDescription(ticket.body);
		errorMessage = '';
		successMessage = '';
	});

	function extractDescription(markdown: string): string {
		const lines = markdown.split('\n');
		const titleLine = lines.findIndex((line) => line.startsWith('# '));
		const start = titleLine >= 0 ? titleLine + 1 : 0;
		let end = lines.findIndex((line, idx) => idx > start && line.startsWith('## Acceptance Criteria'));
		if (end < 0) end = lines.length;
		return lines.slice(start, end).join('\n').trim();
	}

	async function saveState() {
		if (!ticket || !stateDraft || stateDraft === ticket.state || savingState) return;
		savingState = true;
		errorMessage = '';
		successMessage = '';
		const result = await onUpdateState(ticket.id, stateDraft);
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
</script>

<Sheet bind:open>
	<SheetContent class="w-[min(72rem,96vw)] max-w-none border-l-slate-200 bg-white/98 p-0 sm:w-[min(64rem,92vw)]">
		{#if ticket}
			<div class="flex h-full flex-col">
				<SheetHeader class="space-y-3 border-b border-slate-200/80 px-6 pt-6 pb-5">
					<div class="flex items-start justify-between gap-3">
						<SheetTitle class="text-xl leading-tight font-semibold text-slate-900">{ticket.id}</SheetTitle>
						<Button variant="outline" size="sm" onclick={() => (open = false)}>Close</Button>
					</div>
					<div class="grid gap-2 sm:grid-cols-[1fr_auto]">
						<input
							class="h-9 rounded-md border border-slate-200 bg-white px-3 text-sm"
							value={titleDraft}
							oninput={(e) => (titleDraft = (e.currentTarget as HTMLInputElement).value)}
							placeholder="Ticket title"
							disabled={savingTitle}
						/>
						<Button size="sm" class="sm:justify-self-end" onclick={saveTitle} disabled={savingTitle}>
							{savingTitle ? 'Saving...' : 'Save title'}
						</Button>
					</div>
					<SheetDescription>
						<div class="flex flex-wrap items-center gap-2">
							<Badge variant="outline">{ticket.state}</Badge>
							<Badge variant="secondary">P{ticket.priority}</Badge>
							<Badge variant="outline">{ticket.created_at}</Badge>
							{#if ticket.parent}<Badge variant="outline">parent: {ticket.parent}</Badge>{/if}
							{#each ticket.labels as label}
								<Badge variant="secondary" class="bg-slate-100 text-slate-700">{label}</Badge>
							{/each}
						</div>
					</SheetDescription>
					<div class="flex flex-wrap items-center gap-2">
						<Select type="single" value={stateDraft} onValueChange={(value: string) => (stateDraft = value)}>
							<SelectTrigger class="w-52 bg-white">
								{stateOptions.find((s: StateOption) => s.key === stateDraft)?.label ?? stateDraft}
							</SelectTrigger>
							<SelectContent>
								{#each stateOptions as state}
									<SelectItem value={state.key}>{state.label}</SelectItem>
								{/each}
							</SelectContent>
						</Select>
						<Button size="sm" variant="secondary" onclick={saveState} disabled={savingState}>
							{savingState ? 'Updating...' : 'Update state'}
						</Button>
					</div>
					{#if errorMessage}
						<p class="text-xs text-red-600">{errorMessage}</p>
					{/if}
					{#if successMessage}
						<p class="text-xs text-emerald-600">{successMessage}</p>
					{/if}
				</SheetHeader>

				<ScrollArea class="h-[calc(100vh-11rem)] px-6 py-5">
					<div class="mb-5 rounded-lg border border-slate-200 bg-slate-50/60 p-3">
						<p class="mb-2 text-xs font-medium tracking-wide text-slate-600 uppercase">Description</p>
						<textarea
							class="min-h-36 w-full rounded-md border border-slate-200 bg-white p-3 text-sm"
							value={descriptionDraft}
							oninput={(e) => (descriptionDraft = (e.currentTarget as HTMLTextAreaElement).value)}
							disabled={savingDescription}
						></textarea>
						<div class="mt-2 flex gap-2">
							<Button size="sm" onclick={saveDescription} disabled={savingDescription}>
								{savingDescription ? 'Saving...' : 'Save description'}
							</Button>
							<Button
								size="sm"
								variant="outline"
								onclick={() => (descriptionDraft = extractDescription(ticket.body))}
								disabled={savingDescription}
							>
								Reset
							</Button>
						</div>
					</div>
					<article class="markdown max-w-none text-sm leading-relaxed text-slate-800">
						{@html html}
					</article>
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
