import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { afterEach, describe, expect, it } from 'vitest';
import { readTicketSummaries, readTickets } from '$lib/server/docket';

const originalDocketDir = process.env.DOCKET_DIR;

afterEach(() => {
	if (originalDocketDir === undefined) {
		delete process.env.DOCKET_DIR;
	} else {
		process.env.DOCKET_DIR = originalDocketDir;
	}
});

describe('readTickets', () => {
	it('loads tickets from .docket/tickets without throwing', () => {
		const repo = fs.mkdtempSync(path.join(os.tmpdir(), 'docket-test-'));
		const ticketsDir = path.join(repo, '.docket', 'tickets');
		fs.mkdirSync(ticketsDir, { recursive: true });
		fs.writeFileSync(
			path.join(ticketsDir, 'TKT-001.md'),
			`---
id: TKT-001
seq: 1
state: todo
priority: 2
labels: [feature]
created_at: "2026-03-14T00:00:00Z"
updated_at: "2026-03-14T00:00:00Z"
---

# TKT-001: First ticket

## Acceptance Criteria
- [ ] Works
`,
			'utf8'
		);

		process.env.DOCKET_DIR = repo;
		const tickets = readTickets();
		expect(tickets.length).toBe(1);
		expect(tickets[0].id).toBe('TKT-001');
		expect(tickets[0].title).toBe('First ticket');
	});

	it('parses YAML-style list frontmatter for labels and children', () => {
		const repo = fs.mkdtempSync(path.join(os.tmpdir(), 'docket-test-'));
		const ticketsDir = path.join(repo, '.docket', 'tickets');
		fs.mkdirSync(ticketsDir, { recursive: true });
		fs.writeFileSync(
			path.join(ticketsDir, 'TKT-900.md'),
			`---
id: TKT-900
seq: 900
state: in-progress
priority: 1
labels:
  - feature
  - ui
children:
  - TKT-901
created_at: "2026-03-15T00:00:00Z"
updated_at: "2026-03-15T00:00:00Z"
---

# TKT-900: Parent
`,
			'utf8'
		);
		fs.writeFileSync(
			path.join(ticketsDir, 'TKT-901.md'),
			`---
id: TKT-901
seq: 901
state: todo
priority: 2
labels:
  - feature
parent: TKT-900
created_at: "2026-03-15T00:00:00Z"
updated_at: "2026-03-15T00:00:00Z"
---

# TKT-901: Child
`,
			'utf8'
		);

		process.env.DOCKET_DIR = repo;
		const summaries = readTicketSummaries();
		const full = readTickets();
		const parentSummary = summaries.find((ticket) => ticket.id === 'TKT-900');
		const parentFull = full.find((ticket) => ticket.id === 'TKT-900');
		const child = summaries.find((ticket) => ticket.id === 'TKT-901');

		expect(parentSummary?.labels).toEqual(['feature', 'ui']);
		expect(parentSummary?.children).toEqual(['TKT-901']);
		expect(parentFull?.children).toEqual(['TKT-901']);
		expect(child?.parent).toBe('TKT-900');
	});

	it('parses rich markdown sections and exposes frontmatter metadata', () => {
		const repo = fs.mkdtempSync(path.join(os.tmpdir(), 'docket-test-'));
		const ticketsDir = path.join(repo, '.docket', 'tickets');
		fs.mkdirSync(ticketsDir, { recursive: true });
		fs.writeFileSync(
			path.join(ticketsDir, 'TKT-950.md'),
			`---
id: TKT-950
seq: 950
state: in-review
priority: 2
labels:
  - feature
blocked_by:
  - TKT-949
created_at: "2026-03-15T00:00:00Z"
updated_at: "2026-03-15T02:00:00Z"
started_at: "2026-03-15T01:00:00Z"
created_by: agent:test
write_hash: abc123
---

# TKT-950: Rich parsing

## Description
Description text.

## Acceptance Criteria
- [x] Works : commit abc123

## Comments
### 2026-03-15T01:30:00Z — agent:test
Completed implementation.

## Handoff
Handoff details.
`,
			'utf8'
		);

		process.env.DOCKET_DIR = repo;
		const ticket = readTickets().find((entry) => entry.id === 'TKT-950');
		expect(ticket).toBeTruthy();
		expect(ticket?.description).toBe('Description text.');
		expect(ticket?.blocked_by).toEqual(['TKT-949']);
		expect(ticket?.created_by).toBe('agent:test');
		expect(ticket?.write_hash).toBe('abc123');
		expect(ticket?.ac).toHaveLength(1);
		expect(ticket?.ac[0].evidence).toBe('commit abc123');
		expect(ticket?.comments).toHaveLength(1);
		expect(ticket?.comments[0]).toMatchObject({
			at: '2026-03-15T01:30:00Z',
			author: 'agent:test',
			body: 'Completed implementation.'
		});
		expect(ticket?.handoff).toBe('Handoff details.');
		expect(ticket?.frontmatter?.created_by).toBe('agent:test');
		expect(ticket?.frontmatter?.blocked_by).toEqual(['TKT-949']);
	});
});
