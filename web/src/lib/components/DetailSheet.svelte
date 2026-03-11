<script lang="ts">
	import { marked } from 'marked';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { ScrollArea } from '$lib/components/ui/scroll-area';
	import {
		Sheet,
		SheetContent,
		SheetDescription,
		SheetHeader,
		SheetTitle
	} from '$lib/components/ui/sheet';
	import type { Ticket } from '$lib/types';

	let { ticket, open = $bindable(false) } = $props<{
		ticket: Ticket | null;
		open: boolean;
	}>();

	const html = $derived(ticket ? marked.parse(ticket.body, { gfm: true }) : '');
</script>

<Sheet bind:open>
	<SheetContent class="w-[min(72rem,96vw)] max-w-none border-l-slate-200 bg-white/98 p-0 sm:w-[min(64rem,92vw)]">
		{#if ticket}
			<div class="flex h-full flex-col">
				<SheetHeader class="space-y-3 border-b border-slate-200/80 px-6 pt-6 pb-5">
					<div class="flex items-start justify-between gap-3">
						<SheetTitle class="text-xl leading-tight font-semibold text-slate-900">{ticket.title}</SheetTitle>
						<Button variant="outline" size="sm" onclick={() => (open = false)}>Close</Button>
					</div>
					<SheetDescription>
						<div class="flex flex-wrap gap-2">
							<Badge variant="outline">{ticket.state}</Badge>
							<Badge variant="secondary">P{ticket.priority}</Badge>
							<Badge variant="outline">{ticket.created_at}</Badge>
							{#if ticket.parent}<Badge variant="outline">parent: {ticket.parent}</Badge>{/if}
							{#each ticket.labels as label}
								<Badge variant="secondary" class="bg-slate-100 text-slate-700">{label}</Badge>
							{/each}
						</div>
					</SheetDescription>
				</SheetHeader>

				<ScrollArea class="h-[calc(100vh-11rem)] px-6 py-5">
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
