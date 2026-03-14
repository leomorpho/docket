<script lang="ts">
	import type { HTMLInputAttributes } from "svelte/elements";
	import { cn, type WithElementRef } from "$lib/utils.js";

	let { ref = $bindable(null), class: className, type = "text", ...restProps }: WithElementRef<HTMLInputAttributes, HTMLInputElement> = $props();
</script>

<input
	bind:this={ref}
	data-slot="input"
	{type}
	class={cn(
		"border-input bg-background ring-offset-background placeholder:text-muted-foreground focus-visible:ring-ring flex h-9 w-full rounded-md border px-3 py-1 text-sm shadow-xs transition-colors file:border-0 file:bg-transparent file:text-sm file:font-medium focus-visible:ring-1 focus-visible:outline-hidden disabled:cursor-not-allowed disabled:opacity-50",
		className
	)}
	{...restProps}
/>
