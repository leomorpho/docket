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
});
