<script lang="ts">
	import type { HTMLTextareaAttributes } from "svelte/elements";
	import { cn, type WithElementRef } from "$lib/utils.js";

	let { ref = $bindable(null), class: className, ...restProps }: WithElementRef<HTMLTextareaAttributes, HTMLTextAreaElement> = $props();
</script>

<textarea
	bind:this={ref}
	data-slot="textarea"
	class={cn(
		"border-input bg-background ring-offset-background placeholder:text-muted-foreground focus-visible:ring-ring flex min-h-16 w-full rounded-md border px-3 py-2 text-sm shadow-xs transition-colors focus-visible:ring-1 focus-visible:outline-hidden disabled:cursor-not-allowed disabled:opacity-50",
		className
	)}
	{...restProps}
></textarea>
