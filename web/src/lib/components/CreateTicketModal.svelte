<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import {
		Sheet,
		SheetContent,
		SheetDescription,
		SheetFooter,
		SheetHeader,
		SheetTitle
	} from '$lib/components/ui/sheet';
	import { Select, SelectContent, SelectItem, SelectTrigger } from '$lib/components/ui/select';
	import type { Config, Ticket } from '$lib/types';

	let {
		open = $bindable(false),
		config,
		projectId,
		onCreate
	} = $props<{
		open: boolean;
		config: Config;
		projectId: string | null;
		onCreate: (ticket: Ticket) => Promise<void>;
	}>();

	let title = $state('');
	let desc = $state('');
	let stateValue = $state('');
	let priority = $state(0);
	let labels = $state('');
	let saving = $state(false);
	let error = $state('');

	$effect(() => {
		if (open) {
			stateValue = config.default_state;
			priority = config.default_priority;
		}
	});

	async function handleSubmit() {
		if (!title.trim()) {
			error = 'Title is required.';
			return;
		}
		saving = true;
		error = '';
		try {
			const response = await fetch('/api/tickets', {
				method: 'POST',
				headers: { 'content-type': 'application/json' },
				body: JSON.stringify({
					title: title.trim(),
					desc: desc.trim(),
					state: stateValue,
					priority,
					labels: labels
						.split(',')
						.map((label: string) => label.trim())
						.filter(Boolean),
					projectId
				})
			});
			const result = await response.json();
			if (result.ok) {
				open = false;
				await onCreate(result.ticket);
				// Reset
				title = '';
				desc = '';
				labels = '';
			} else {
				error = result.error || 'Failed to create ticket.';
			}
		} catch (e: unknown) {
			const message = e instanceof Error ? e.message : 'Failed to create ticket.';
			error = message;
		} finally {
			saving = false;
		}
	}
</script>

<Sheet bind:open>
	<SheetContent class="w-[min(42rem,96vw)] max-w-none border-l-border bg-card/98 p-0 sm:w-[min(36rem,92vw)]">
		<div class="flex h-full flex-col">
			<SheetHeader class="space-y-2 border-b border-border/80 px-6 pt-6 pb-5">
				<SheetTitle class="text-xl leading-tight font-semibold text-foreground">Create Ticket</SheetTitle>
				<SheetDescription>
					Fill in the ticket details. Fields are saved when you click create.
				</SheetDescription>
			</SheetHeader>

			<div class="grid gap-4 px-6 py-5">
				<label class="grid gap-2">
					<span class="text-sm font-medium text-foreground">Title</span>
					<input
						class="h-9 rounded-md border border-input bg-background px-3 text-sm"
						value={title}
						oninput={(e) => (title = (e.currentTarget as HTMLInputElement).value)}
						placeholder="A brief summary of the task"
					/>
				</label>

				<label class="grid gap-2">
					<span class="text-sm font-medium text-foreground">Description</span>
					<textarea
						class="min-h-24 rounded-md border border-input bg-background px-3 py-2 text-sm"
						value={desc}
						oninput={(e) => (desc = (e.currentTarget as HTMLTextAreaElement).value)}
						placeholder="Detailed explanation..."
					></textarea>
				</label>

				<div class="grid grid-cols-2 gap-4">
					<div class="grid gap-2">
						<span class="text-sm font-medium text-foreground">State</span>
						<Select type="single" value={stateValue} onValueChange={(v: string) => (stateValue = v)}>
							<SelectTrigger class="bg-background">
								{stateValue}
							</SelectTrigger>
							<SelectContent>
								{#each Object.keys(config.states) as s}
									<SelectItem value={s}>{config.states[s].label}</SelectItem>
								{/each}
							</SelectContent>
						</Select>
					</div>
					<label class="grid gap-2">
						<span class="text-sm font-medium text-foreground">Priority</span>
						<input
							type="number"
							class="h-9 rounded-md border border-input bg-background px-3 text-sm"
							value={priority}
							oninput={(e) => (priority = Number((e.currentTarget as HTMLInputElement).value || 0))}
						/>
					</label>
				</div>

				<label class="grid gap-2">
					<span class="text-sm font-medium text-foreground">Labels (comma-separated)</span>
					<input
						class="h-9 rounded-md border border-input bg-background px-3 text-sm"
						value={labels}
						oninput={(e) => (labels = (e.currentTarget as HTMLInputElement).value)}
						placeholder="feature, ui, bug"
					/>
				</label>

				{#if error}
					<p class="text-xs text-red-600">{error}</p>
				{/if}
			</div>

			<SheetFooter class="border-t border-border/80 px-6 py-4">
				<Button variant="outline" onclick={() => (open = false)}>Cancel</Button>
				<Button type="button" onclick={handleSubmit} disabled={saving}>
					{saving ? 'Creating...' : 'Create Ticket'}
				</Button>
			</SheetFooter>
		</div>
	</SheetContent>
</Sheet>
