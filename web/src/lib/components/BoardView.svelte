<script lang="ts">
	import { createEventDispatcher } from 'svelte';
	import TicketCard from '$lib/components/TicketCard.svelte';
	import type { Ticket } from '$lib/types';

	type Column = {
		key: string;
		label: string;
		column: number;
		tickets: Ticket[];
	};

	let { columns } = $props<{ columns: Column[] }>();
	const dispatch = createEventDispatcher<{ select: { ticket: Ticket } }>();
</script>

<section class="board">
	{#each columns as col}
		<div class="column">
			<header class="col-header">
				<h2>{col.label}</h2>
				<span>{col.tickets.length}</span>
			</header>
			<div class="col-body">
				{#each col.tickets as ticket}
					<TicketCard {ticket} on:select={(e: CustomEvent<{ ticket: Ticket }>) => dispatch('select', e.detail)} />
				{/each}
			</div>
		</div>
	{/each}
</section>

<style>
	.board {
		display: flex;
		gap: 0.85rem;
		overflow-x: auto;
		padding-bottom: 0.5rem;
		min-height: 0;
	}

	.column {
		min-width: 300px;
		max-width: 320px;
		flex: 0 0 320px;
		border: 1px solid #dae4f1;
		border-radius: 14px;
		background: linear-gradient(180deg, #f8fbff 0%, #f3f7fc 100%);
		display: flex;
		flex-direction: column;
		max-height: calc(100vh - 230px);
	}

	.col-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
		padding: 0.7rem 0.8rem;
		border-bottom: 1px solid #dce6f2;
	}

	.col-header h2 {
		margin: 0;
		font-size: 0.85rem;
		letter-spacing: 0.03em;
		text-transform: uppercase;
	}

	.col-header span {
		font-size: 0.78rem;
		background: #e5edf8;
		border-radius: 999px;
		padding: 0.1rem 0.45rem;
	}

	.col-body {
		padding: 0.6rem;
		display: flex;
		flex-direction: column;
		gap: 0.55rem;
		overflow-y: auto;
	}
</style>
