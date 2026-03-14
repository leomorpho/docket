import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { afterEach, describe, expect, it, vi } from 'vitest';

const originalDocketDir = process.env.DOCKET_DIR;

afterEach(() => {
	if (originalDocketDir === undefined) {
		delete process.env.DOCKET_DIR;
	} else {
		process.env.DOCKET_DIR = originalDocketDir;
	}
	vi.resetModules();
});

function seedProjectRepo(baseName: string): string {
	const repo = fs.mkdtempSync(path.join(os.tmpdir(), `${baseName}-`));
	const docketDir = path.join(repo, '.docket');
	const ticketsDir = path.join(docketDir, 'tickets');
	fs.mkdirSync(ticketsDir, { recursive: true });

	fs.writeFileSync(
		path.join(docketDir, 'config.json'),
		JSON.stringify(
			{
				states: {
					backlog: { label: 'Backlog', open: true, column: 0, next: ['todo'] },
					todo: { label: 'To Do', open: true, column: 1, next: ['in-progress'] },
					'in-progress': { label: 'In Progress', open: true, column: 2, next: ['in-review'] },
					'in-review': { label: 'In Review', open: true, column: 3, next: ['done'] },
					done: { label: 'Done', open: false, column: 4, next: [] }
				},
				default_state: 'backlog',
				default_priority: 5,
				labels: ['feature']
			},
			null,
			2
		),
		'utf8'
	);
	fs.writeFileSync(
		path.join(docketDir, 'relations.json'),
		JSON.stringify({ relations: [{ from: 'TKT-001', to: 'TKT-002', relation: 'blocked_by' }] }, null, 2),
		'utf8'
	);
	fs.writeFileSync(
		path.join(ticketsDir, 'TKT-001.md'),
		`---
id: TKT-001
seq: 1
state: todo
priority: 1
labels: [feature]
created_at: "2026-03-14T00:00:00Z"
updated_at: "2026-03-14T00:00:00Z"
---

# TKT-001: Integration test ticket

## Acceptance Criteria
- [ ] Works end to end
`,
		'utf8'
	);

	return repo;
}

describe('UI integration: project registration + page load', () => {
	it('loads registered project data through +page.server', async () => {
		const repo = seedProjectRepo('docket-ui-project');
		process.env.DOCKET_DIR = repo;

		const { POST } = await import('./api/projects/+server');
		const registerRes = await POST({
			request: new Request('http://localhost/api/projects', {
				method: 'POST',
				headers: { 'content-type': 'application/json' },
				body: JSON.stringify({ dir: repo })
			})
		} as never);
		const registerBody = (await registerRes.json()) as {
			ok: boolean;
			project: { id: string };
		};
		expect(registerBody.ok).toBe(true);
		expect(registerBody.project.id).toBeTruthy();

		const { load } = await import('./+page.server');
		const pageData = (await load({
			url: new URL(`http://localhost/?project=${registerBody.project.id}`)
		} as never)) as {
			activeProjectId: string | null;
			projects: Array<{ id: string }>;
			tickets: Array<{ id: string }>;
			config: { default_priority: number };
			relations: Array<{ relation: string }>;
		};

		expect(pageData.activeProjectId).toBe(registerBody.project.id);
		expect(pageData.projects.length).toBeGreaterThan(0);
		expect(pageData.tickets.length).toBe(1);
		expect(pageData.tickets[0].id).toBe('TKT-001');
		expect(pageData.config.default_priority).toBe(5);
		expect(pageData.relations.length).toBe(1);
		expect(pageData.relations[0].relation).toBe('blocked_by');
	});
});
