import type { Ticket } from '$lib/types';

function normalizeLinkedIDs(value: unknown): string[] {
	if (!Array.isArray(value)) return [];
	return value
		.map((item) => String(item).trim())
		.filter(Boolean);
}

function compareTickets(a: Ticket, b: Ticket): number {
	if (a.priority !== b.priority) return a.priority - b.priority;
	if (a.seq !== b.seq) return a.seq - b.seq;
	return a.id.localeCompare(b.id);
}

export function buildChildrenByParent(tickets: Ticket[]): Map<string, Ticket[]> {
	const byID = new Map<string, Ticket>();
	for (const ticket of tickets) {
		byID.set(ticket.id, ticket);
	}

	const map = new Map<string, Ticket[]>();
	const seen = new Map<string, Set<string>>();

	const addEdge = (parentID: string, childID: string) => {
		if (!parentID || !childID || parentID === childID) return;
		const child = byID.get(childID);
		if (!child) return;

		let known = seen.get(parentID);
		if (!known) {
			known = new Set<string>();
			seen.set(parentID, known);
		}
		if (known.has(childID)) return;
		known.add(childID);

		const children = map.get(parentID) ?? [];
		children.push(child);
		map.set(parentID, children);
	};

	for (const ticket of tickets) {
		if (ticket.parent) {
			addEdge(ticket.parent, ticket.id);
		}
	}

	for (const ticket of tickets) {
		for (const childID of normalizeLinkedIDs(ticket.children)) {
			addEdge(ticket.id, childID);
		}
	}

	for (const children of map.values()) {
		children.sort(compareTickets);
	}

	return map;
}

export function buildChildCounts(tickets: Ticket[]): Record<string, number> {
	const byParent = buildChildrenByParent(tickets);
	const counts: Record<string, number> = {};
	for (const [parentID, children] of byParent.entries()) {
		counts[parentID] = children.length;
	}
	return counts;
}
