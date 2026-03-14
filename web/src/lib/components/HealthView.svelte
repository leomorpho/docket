<script lang="ts">
	import { onMount } from 'svelte';
	import type { ProjectHealth, Finding } from '$lib/types';
	import { Card, CardContent, CardHeader, CardTitle } from '$lib/components/ui/card';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { createEventDispatcher } from 'svelte';

	let { projectId } = $props<{ projectId: string | null }>();
	const dispatch = createEventDispatcher();

	let health = $state<ProjectHealth | null>(null);
	let loading = $state(false);
	let error = $state<string | null>(null);

	async function fetchHealth() {
		loading = true;
		error = null;
		try {
			const url = projectId ? `/api/projects/health?projectId=${projectId}` : '/api/projects/health';
			const response = await fetch(url);
			const data = await response.json();
			if (data.ok) {
				health = data.health;
			} else {
				error = data.error || 'Failed to fetch health metrics.';
			}
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'Failed to fetch health metrics.';
		} finally {
			loading = false;
		}
	}

	onMount(() => {
		fetchHealth();
	});

	$effect(() => {
		if (projectId) fetchHealth();
	});
</script>

<div class="space-y-6">
	<div class="flex items-center justify-between">
		<h2 class="text-xl font-semibold">Project Health</h2>
		<Button variant="outline" size="sm" onclick={fetchHealth} disabled={loading}>
			{loading ? 'Running...' : 'Run Doctor'}
		</Button>
	</div>

	{#if error}
		<Card class="border-red-200 bg-red-50 text-red-800">
			<CardContent class="pt-6">
				<p>{error}</p>
			</CardContent>
		</Card>
	{/if}

	{#if health}
		<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
			<Card>
				<CardHeader class="pb-2">
					<CardTitle class="text-sm font-medium text-muted-foreground">Total Tickets</CardTitle>
				</CardHeader>
				<CardContent>
					<div class="text-2xl font-bold">{health.ticketCount}</div>
				</CardContent>
			</Card>
			<Card>
				<CardHeader class="pb-2">
					<CardTitle class="text-sm font-medium text-muted-foreground">Invalid Signatures</CardTitle>
				</CardHeader>
				<CardContent>
					<div class="text-2xl font-bold {health.invalidSignatures.length > 0 ? 'text-red-600' : ''}">
						{health.invalidSignatures.length}
					</div>
				</CardContent>
			</Card>
			<Card>
				<CardHeader class="pb-2">
					<CardTitle class="text-sm font-medium text-muted-foreground">Stale Tickets</CardTitle>
				</CardHeader>
				<CardContent>
					<div class="text-2xl font-bold">{health.staleTickets.length}</div>
				</CardContent>
			</Card>
			<Card>
				<CardHeader class="pb-2">
					<CardTitle class="text-sm font-medium text-muted-foreground">Health Score</CardTitle>
				</CardHeader>
				<CardContent>
					<div class="text-2xl font-bold text-green-600">
						{Math.max(0, 100 - (health.findings.length * 5))}%
					</div>
				</CardContent>
			</Card>
		</div>

		<div class="grid gap-4 sm:grid-cols-2">
			<Card>
				<CardHeader>
					<CardTitle>Performance</CardTitle>
				</CardHeader>
				<CardContent>
					<div class="space-y-4">
						<div class="flex justify-between items-center">
							<span class="text-sm text-muted-foreground">Avg. Cycle Time</span>
							<span class="font-semibold">{health.avgCycleTime.toFixed(1)} days</span>
						</div>
						<div class="flex justify-between items-center">
							<span class="text-sm text-muted-foreground">Tickets in Progress</span>
							<Badge>{health.findings.filter((f) => f.rule === 'V001' && (f.message ?? '').includes('in-progress')).length}</Badge>
						</div>
					</div>
				</CardContent>
			</Card>
		</div>

		<Card>
			<CardHeader>
				<CardTitle>Detailed Findings</CardTitle>
			</CardHeader>
			<CardContent>
				{#if health.findings.length === 0}
					<p class="text-sm text-muted-foreground text-center py-8">No issues found. Your project is in perfect health!</p>
				{:else}
					<div class="divide-y divide-border">
						{#each health.findings as finding}
							<div class="flex items-start justify-between py-3 gap-4">
								<div class="space-y-1">
									<div class="flex items-center gap-2">
										<Badge variant={finding.severity === 'error' ? 'destructive' : 'outline'}>
											{finding.ticketId}
										</Badge>
										<span class="text-sm font-medium">{finding.rule}</span>
									</div>
									<p class="text-sm text-muted-foreground">{finding.message}</p>
								</div>
								<Button variant="ghost" size="sm" onclick={() => dispatch('select', { id: finding.ticketId })}>
									View
								</Button>
							</div>
						{/each}
					</div>
				{/if}
			</CardContent>
		</Card>
	{/if}
</div>
