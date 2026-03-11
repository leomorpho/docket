<script lang="ts">
	import { createEventDispatcher } from 'svelte';
	import type { Ticket } from '$lib/types';

	let { ticket } = $props<{ ticket: Ticket }>();
	const dispatch = createEventDispatcher<{ select: { ticket: Ticket } }>();

	function select() {
		dispatch('select', { ticket });
	}
</script>

<button class="card" type="button" onclick={select}>
	<div class="top">
		<span class="id">{ticket.id}</span>
		<span class="priority">P{ticket.priority}</span>
	</div>
	<h3 class="title">{ticket.title}</h3>
	{#if ticket.labels.length > 0}
		<div class="labels">
			{#each ticket.labels as label}
				<span class="label">{label}</span>
			{/each}
		</div>
	{/if}
</button>

<style>
	.card {
		width: 100%;
		border: 1px solid #dbe3ef;
		border-radius: 12px;
		background: #fff;
		padding: 0.75rem;
		text-align: left;
		cursor: pointer;
		box-shadow: 0 1px 2px rgba(15, 23, 42, 0.08);
	}

	.card:hover {
		border-color: #9fb6d8;
	}

	.top {
		display: flex;
		justify-content: space-between;
		gap: 0.5rem;
		font-size: 0.75rem;
		margin-bottom: 0.5rem;
	}

	.id,
	.priority {
		background: #eef3fb;
		padding: 0.15rem 0.4rem;
		border-radius: 999px;
	}

	.title {
		margin: 0;
		font-size: 0.95rem;
		line-height: 1.3;
		display: -webkit-box;
		line-clamp: 2;
		-webkit-line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}

	.labels {
		display: flex;
		flex-wrap: wrap;
		gap: 0.35rem;
		margin-top: 0.6rem;
	}

	.label {
		font-size: 0.72rem;
		background: #f5f7fb;
		border: 1px solid #e3e9f4;
		border-radius: 999px;
		padding: 0.1rem 0.45rem;
	}
</style>
