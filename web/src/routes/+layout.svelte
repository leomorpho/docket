<script lang="ts">
	import favicon from '$lib/assets/favicon.svg';
	import '../app.css';
	import { onMount } from 'svelte';

	let { children } = $props();
	let theme = $state<'light' | 'dark' | 'system'>('system');

	function applyTheme(t: 'light' | 'dark' | 'system') {
		const root = window.document.documentElement;
		root.classList.remove('light', 'dark');

		if (t === 'system') {
			const systemTheme = window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
			root.classList.add(systemTheme);
		} else {
			root.classList.add(t);
		}
	}

	onMount(() => {
		const persisted = localStorage.getItem('docket_theme') as 'light' | 'dark' | 'system';
		if (persisted) {
			theme = persisted;
		}
		applyTheme(theme);

		const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
		const handleChange = () => {
			if (theme === 'system') applyTheme('system');
		};
		mediaQuery.addEventListener('change', handleChange);
		return () => mediaQuery.removeEventListener('change', handleChange);
	});

	$effect(() => {
		applyTheme(theme);
		localStorage.setItem('docket_theme', theme);
	});
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
</svelte:head>

<div class="fixed top-4 right-4 z-50">
	<div class="flex items-center gap-1 p-1 bg-card/80 backdrop-blur-sm border border-border rounded-full shadow-sm">
		<button 
			class="px-2 py-1 text-[10px] font-bold rounded-full transition-colors {theme === 'light' ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:bg-accent'}"
			onclick={() => theme = 'light'}
		>LIGHT</button>
		<button 
			class="px-2 py-1 text-[10px] font-bold rounded-full transition-colors {theme === 'dark' ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:bg-accent'}"
			onclick={() => theme = 'dark'}
		>DARK</button>
		<button 
			class="px-2 py-1 text-[10px] font-bold rounded-full transition-colors {theme === 'system' ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:bg-accent'}"
			onclick={() => theme = 'system'}
		>AUTO</button>
	</div>
</div>

{@render children()}
