<script lang="ts">
	import { createEventDispatcher } from 'svelte';
	import { Badge } from '$lib/components/ui/badge';
	import TicketHierarchyTree from '$lib/components/TicketHierarchyTree.svelte';

	let { nodes = [] } = $props<{ nodes?: any[] }>();

	const dispatch = createEventDispatcher<{ select: { id: string } }>();

	function select(id: string) {
		dispatch('select', { id });
	}

	function forwardSelect(event: CustomEvent<{ id: string }>) {
		dispatch('select', event.detail);
	}
</script>

<div class="space-y-2">
	{#each nodes as node}
		<div class="rounded-md border border-border bg-background p-2">
			<button
				type="button"
				class="w-full rounded-sm p-1 text-left transition-colors hover:bg-muted/40"
				onclick={() => select(node.ticket.id)}
			>
				<div class="flex items-start justify-between gap-2">
					<div class="min-w-0">
						<p class="font-mono text-xs font-semibold text-muted-foreground">{node.ticket.id}</p>
						<p class="line-clamp-2 text-sm font-medium text-foreground">{node.ticket.title}</p>
					</div>
					<div class="flex shrink-0 items-center gap-1">
						<Badge variant="outline" class="text-[10px]">{node.ticket.state}</Badge>
						{#if node.children.length > 0}
							<Badge variant="secondary" class="text-[10px]">
								{node.children.length} child{node.children.length === 1 ? '' : 'ren'}
							</Badge>
						{/if}
					</div>
				</div>
			</button>

			{#if node.children.length > 0}
				<div class="mt-2 border-l border-border pl-3">
					<TicketHierarchyTree nodes={node.children} on:select={forwardSelect} />
				</div>
			{/if}
		</div>
	{/each}
</div>
