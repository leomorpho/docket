<script lang="ts">
	type Layout = 'inline' | 'stacked';

	let { value, layout = 'stacked', fallback = '—' } = $props<{
		value?: string;
		layout?: Layout;
		fallback?: string;
	}>();

	function parseDate(raw?: string): Date | null {
		if (!raw) return null;
		const parsed = new Date(raw);
		return Number.isNaN(parsed.getTime()) ? null : parsed;
	}

	function formatRelative(value: Date, now: Date): string {
		const diffMs = value.getTime() - now.getTime();
		const absMs = Math.abs(diffMs);

		if (absMs < 45_000) {
			return diffMs <= 0 ? 'just now' : 'in a moment';
		}

		const minute = 60_000;
		const hour = 60 * minute;
		const day = 24 * hour;
		const week = 7 * day;
		const month = 30 * day;
		const year = 365 * day;

		const rtf = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' });
		if (absMs < hour) {
			return rtf.format(Math.round(diffMs / minute), 'minute');
		}
		if (absMs < day) {
			return rtf.format(Math.round(diffMs / hour), 'hour');
		}
		if (absMs < week) {
			return rtf.format(Math.round(diffMs / day), 'day');
		}
		if (absMs < month) {
			return rtf.format(Math.round(diffMs / week), 'week');
		}
		if (absMs < year) {
			return rtf.format(Math.round(diffMs / month), 'month');
		}
		return rtf.format(Math.round(diffMs / year), 'year');
	}

	function formatCalendar(value: Date, now: Date): string {
		const time = value.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' });
		const todayStart = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime();
		const valueStart = new Date(value.getFullYear(), value.getMonth(), value.getDate()).getTime();
		const dayDiff = Math.round((valueStart - todayStart) / 86_400_000);

		if (dayDiff === 0) return `Today at ${time}`;
		if (dayDiff === -1) return `Yesterday at ${time}`;
		if (dayDiff === 1) return `Tomorrow at ${time}`;
		if (Math.abs(dayDiff) < 7) {
			return `${value.toLocaleDateString([], { weekday: 'long' })} at ${time}`;
		}
		return value.toLocaleString([], { dateStyle: 'medium', timeStyle: 'short' });
	}

	const parsed = $derived.by(() => parseDate(value));
	const now = $derived.by(() => new Date());
	const relative = $derived.by(() => (parsed ? formatRelative(parsed, now) : fallback));
	const calendar = $derived.by(() => (parsed ? formatCalendar(parsed, now) : fallback));
	const absolute = $derived.by(() =>
		parsed ? parsed.toLocaleString([], { dateStyle: 'full', timeStyle: 'long' }) : fallback
	);
</script>

{#if layout === 'inline'}
	<time datetime={value} title={absolute} class="text-xs text-muted-foreground">
		{relative}
	</time>
{:else}
	<div class="flex flex-col">
		<time datetime={value} title={absolute} class="text-sm font-medium text-foreground">{relative}</time>
		<p class="text-xs text-muted-foreground">{calendar}</p>
	</div>
{/if}
