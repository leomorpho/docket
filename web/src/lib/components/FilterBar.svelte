<script lang="ts">
	import { createEventDispatcher } from 'svelte';
	import { Button } from '$lib/components/ui/button';
	import { Select, SelectContent, SelectItem, SelectTrigger } from '$lib/components/ui/select';
	import { ToggleGroup, ToggleGroupItem } from '$lib/components/ui/toggle-group';

	let {
		stateOptions,
		labelOptions,
		selectedStates,
		selectedLabel,
		maxPriority
	} = $props<{
		stateOptions: { key: string; label: string }[];
		labelOptions: string[];
		selectedStates: Set<string>;
		selectedLabel: string;
		maxPriority: number;
	}>();

	const dispatch = createEventDispatcher<{
		toggleState: { key: string };
		label: { value: string };
		priority: { value: number };
		clear: undefined;
	}>();

	function labelDisplayText(): string {
		return selectedLabel || 'All labels';
	}

	function onLabelChange(value: string) {
		dispatch('label', { value: value === '__all' ? '' : value });
	}
</script>

<div class="rounded-xl border border-slate-200/80 bg-slate-50/60 p-3">
	<div class="flex flex-wrap items-center gap-2">
		{#each stateOptions as state}
			<Button
				variant={selectedStates.has(state.key) ? 'secondary' : 'outline'}
				size="sm"
				onclick={() => dispatch('toggleState', { key: state.key })}
			>
				{state.label}
			</Button>
		{/each}
	</div>

	<div class="mt-3 flex flex-wrap items-end gap-3">
		<div class="flex flex-col gap-1">
			<p class="text-xs font-medium text-muted-foreground">Label</p>
			<Select
				type="single"
				value={selectedLabel || '__all'}
				onValueChange={(value: string) => onLabelChange(value || '__all')}
			>
				<SelectTrigger class="w-48 bg-white">{labelDisplayText()}</SelectTrigger>
				<SelectContent>
					<SelectItem value="__all">All labels</SelectItem>
					{#each labelOptions as label}
						<SelectItem value={label}>{label}</SelectItem>
					{/each}
				</SelectContent>
			</Select>
		</div>

		<div class="flex flex-col gap-1">
			<p class="text-xs font-medium text-muted-foreground">Max Priority</p>
			<ToggleGroup
				type="single"
				class="rounded-md bg-white p-1"
				value={maxPriority === 0 ? 'all' : String(maxPriority)}
				onValueChange={(value) =>
					dispatch('priority', {
						value: value === 'all' || !value ? 0 : Number(value)
					})}
			>
				<ToggleGroupItem size="sm" value="all">All</ToggleGroupItem>
				{#each [1, 2, 3, 4, 5] as p}
					<ToggleGroupItem size="sm" value={String(p)}>≤P{p}</ToggleGroupItem>
				{/each}
			</ToggleGroup>
		</div>

		<div class="ml-auto">
			<Button variant="outline" size="sm" class="bg-white" onclick={() => dispatch('clear')}>
				Clear filters
			</Button>
		</div>
	</div>
</div>
