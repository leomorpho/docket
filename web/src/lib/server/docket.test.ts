import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { afterEach, describe, expect, it } from 'vitest';
import { readTickets } from '$lib/server/docket';

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
});
