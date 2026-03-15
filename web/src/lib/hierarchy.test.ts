import { describe, expect, it } from 'vitest';
import { buildChildCounts, buildChildrenByParent } from '$lib/hierarchy';
import type { Ticket } from '$lib/types';

function makeTicket(
	id: string,
	overrides: Partial<Ticket> = {}
): Ticket {
	return {
		id,
		seq: Number(id.replace('TKT-', '')),
		state: 'todo',
		priority: 1,
		labels: [],
		title: id,
		created_at: '2026-03-15T00:00:00Z',
		updated_at: '2026-03-15T00:00:00Z',
		ac: [],
		plan: [],
		comments: [],
		body: '',
		...overrides
	};
}

describe('hierarchy helpers', () => {
	it('builds child maps from parent pointers and linked children without duplicates', () => {
		const tickets: Ticket[] = [
			makeTicket('TKT-900', { children: ['TKT-901', 'TKT-902'] }),
			makeTicket('TKT-901', { parent: 'TKT-900', priority: 2 }),
			makeTicket('TKT-902', { priority: 3 }),
			makeTicket('TKT-903', { parent: 'TKT-901', priority: 4 })
		];

		const byParent = buildChildrenByParent(tickets);
		const rootChildren = byParent.get('TKT-900') ?? [];
		const nestedChildren = byParent.get('TKT-901') ?? [];

		expect(rootChildren.map((ticket) => ticket.id)).toEqual(['TKT-901', 'TKT-902']);
		expect(nestedChildren.map((ticket) => ticket.id)).toEqual(['TKT-903']);
		expect(buildChildCounts(tickets)).toEqual({
			'TKT-900': 2,
			'TKT-901': 1
		});
	});
});
