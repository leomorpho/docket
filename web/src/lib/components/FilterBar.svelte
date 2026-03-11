<script lang="ts">
	import { createEventDispatcher } from 'svelte';

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
</script>

<div class="bar">
	<div class="group states">
		{#each stateOptions as state}
			<button
				type="button"
				class:active={selectedStates.has(state.key)}
				onclick={() => dispatch('toggleState', { key: state.key })}
			>
				{state.label}
			</button>
		{/each}
	</div>

	<div class="group">
		<label>
			Label
			<select value={selectedLabel} onchange={(e) => dispatch('label', { value: (e.currentTarget as HTMLSelectElement).value })}>
				<option value="">All</option>
				{#each labelOptions as label}
					<option value={label}>{label}</option>
				{/each}
			</select>
		</label>
	</div>

	<div class="group">
		<label>
			Max Priority
			<div class="priorities">
				{#each [0, 1, 2, 3, 4, 5] as p}
					<button
						type="button"
						class:active={maxPriority === p}
						onclick={() => dispatch('priority', { value: p })}
					>
						{p === 0 ? 'All' : `≤P${p}`}
					</button>
				{/each}
			</div>
		</label>
	</div>

	<button class="clear" type="button" onclick={() => dispatch('clear')}>Clear filters</button>
</div>

<style>
	.bar {
		display: flex;
		flex-wrap: wrap;
		gap: 0.7rem;
		padding: 0.65rem;
		border: 1px solid #d6e0ee;
		border-radius: 12px;
		background: #f8fbff;
	}

	.group {
		display: flex;
		align-items: center;
		gap: 0.4rem;
	}

	.states {
		flex-wrap: wrap;
	}

	button,
	select {
		border: 1px solid #cedaeb;
		background: #fff;
		border-radius: 8px;
		padding: 0.25rem 0.5rem;
		cursor: pointer;
	}

	button.active {
		background: #e6eefb;
		border-color: #95acd0;
	}

	.priorities {
		display: inline-flex;
		gap: 0.3rem;
	}

	.clear {
		margin-left: auto;
	}
</style>
