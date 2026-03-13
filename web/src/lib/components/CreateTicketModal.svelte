<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import {
		Dialog,
		DialogContent,
		DialogDescription,
		DialogFooter,
		DialogHeader,
		DialogTitle
	} from '$lib/components/ui/dialog';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Textarea } from '$lib/components/ui/textarea';
	import { Select, SelectContent, SelectItem, SelectTrigger } from '$lib/components/ui/select';
	import type { Config } from '$lib/types';

	let {
		open = $bindable(false),
		config,
		projectId,
		onCreate
	} = $props<{
		open: boolean;
		config: Config;
		projectId: string | null;
		onCreate: (ticket: any) => Promise<void>;
	}>();

	let title = $state('');
	let desc = $state('');
	let state = $state(config.default_state);
	let priority = $state(config.default_priority);
	let labels = $state('');
	let saving = $state(false);
	let error = $state('');

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
					state,
					priority,
					labels: labels.split(',').map(l => l.trim()).filter(Boolean),
					projectId
				})
			});
			const result = await response.json();
			if (result.ok) {
				await onCreate(result.ticket);
				open = false;
				// Reset
				title = '';
				desc = '';
				state = config.default_state;
				priority = config.default_priority;
				labels = '';
			} else {
				error = result.error || 'Failed to create ticket.';
			}
		} catch (e: any) {
			error = e.message;
		} finally {
			saving = false;
		}
	}
</script>

<Dialog bind:open>
	<DialogContent class="sm:max-w-[525px]">
		<DialogHeader>
			<DialogTitle>Create New Ticket</DialogTitle>
			<DialogDescription>
				Fill in the details for the new ticket. Click save when you're done.
			</DialogDescription>
		</DialogHeader>
		<div class="grid gap-4 py-4">
			<div class="grid gap-2">
				<Label for="title">Title</Label>
				<Input id="title" bind:value={title} placeholder="A brief summary of the task" />
			</div>
			<div class="grid gap-2">
				<Label for="desc">Description</Label>
				<Textarea id="desc" bind:value={desc} placeholder="Detailed explanation..." />
			</div>
			<div class="grid grid-cols-2 gap-4">
				<div class="grid gap-2">
					<Label>State</Label>
					<Select type="single" value={state} onValueChange={(v) => (state = v)}>
						<SelectTrigger>{state}</SelectTrigger>
						<SelectContent>
							{#each Object.keys(config.states) as s}
								<SelectItem value={s}>{config.states[s].label}</SelectItem>
							{/each}
						</SelectContent>
					</Select>
				</div>
				<div class="grid gap-2">
					<Label for="priority">Priority</Label>
					<Input id="priority" type="number" bind:value={priority} />
				</div>
			</div>
			<div class="grid gap-2">
				<Label for="labels">Labels (comma-separated)</Label>
				<Input id="labels" bind:value={labels} placeholder="feature, ui, bug" />
			</div>
			{#if error}
				<p class="text-xs text-red-600">{error}</p>
			{/if}
		</div>
		<DialogFooter>
			<Button variant="outline" onclick={() => (open = false)}>Cancel</Button>
			<Button type="submit" onclick={handleSubmit} disabled={saving}>
				{saving ? 'Creating...' : 'Create Ticket'}
			</Button>
		</DialogFooter>
	</DialogContent>
</Dialog>
