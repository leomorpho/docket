<script lang="ts">
	import { marked } from 'marked';
	import type { Ticket } from '$lib/types';

	let { ticket, open = false, onClose } = $props<{
		ticket: Ticket | null;
		open: boolean;
		onClose: () => void;
	}>();

	const html = $derived(ticket ? marked.parse(ticket.body, { gfm: true }) : '');

	function onKeydown(e: KeyboardEvent) {
		if (open && e.key === 'Escape') onClose();
	}
</script>

<svelte:window onkeydown={onKeydown} />

{#if open && ticket}
	<button class="overlay" onclick={onClose} aria-label="Close details"></button>
	<section class="sheet" aria-modal="true">
		<header class="header">
			<h2>{ticket.title}</h2>
			<button type="button" class="close" onclick={onClose}>Close</button>
		</header>
		<div class="meta">
			<span>{ticket.state}</span>
			<span>P{ticket.priority}</span>
			<span>{ticket.created_at}</span>
			{#if ticket.parent}<span>parent: {ticket.parent}</span>{/if}
			{#if ticket.labels.length > 0}<span>labels: {ticket.labels.join(', ')}</span>{/if}
		</div>
		<article class="markdown">
			{@html html}
		</article>
	</section>
{/if}

<style>
	.overlay {
		position: fixed;
		inset: 0;
		border: 0;
		background: rgba(15, 23, 42, 0.44);
		z-index: 40;
	}

	.sheet {
		position: fixed;
		right: 0;
		top: 0;
		bottom: 0;
		width: min(58rem, 92vw);
		background: #fff;
		z-index: 50;
		padding: 1rem;
		display: flex;
		flex-direction: column;
		gap: 0.8rem;
		box-shadow: -10px 0 28px rgba(15, 23, 42, 0.25);
		overflow: auto;
	}

	.header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.8rem;
	}

	.header h2 {
		margin: 0;
	}

	.close {
		border: 1px solid #d4dfed;
		background: #f7faff;
		border-radius: 8px;
		padding: 0.3rem 0.65rem;
		cursor: pointer;
	}

	.meta {
		display: flex;
		flex-wrap: wrap;
		gap: 0.45rem;
	}

	.meta span {
		background: #f2f6fd;
		border: 1px solid #dde6f5;
		border-radius: 999px;
		padding: 0.12rem 0.5rem;
		font-size: 0.78rem;
	}

	.markdown :global(pre) {
		background: #0f172a;
		color: #e2e8f0;
		padding: 0.8rem;
		border-radius: 8px;
		overflow: auto;
	}

	.markdown :global(code) {
		font-family: "JetBrains Mono", ui-monospace, SFMono-Regular, Menlo, monospace;
	}
</style>
