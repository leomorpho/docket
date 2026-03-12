import fs from 'node:fs';
import path from 'node:path';
import type { Config, Ticket } from '$lib/types';
import { getProject } from '$lib/server/registry';

type FrontmatterTicket = Omit<Ticket, 'title' | 'body'>;

const defaultConfig: Config = {
	states: {
		backlog: { label: 'Backlog', open: true, column: 0, next: ['todo', 'archived'] },
		todo: { label: 'To Do', open: true, column: 1, next: ['in-progress', 'backlog', 'archived'] },
		'in-progress': { label: 'In Progress', open: true, column: 2, next: ['in-review', 'todo', 'archived'] },
		'in-review': { label: 'In Review', open: true, column: 3, next: ['done', 'in-progress', 'archived'] },
		done: { label: 'Done', open: false, column: 4, next: ['archived', 'in-progress'] },
		archived: { label: 'Archived', open: false, column: 5, next: ['backlog'] }
	},
	default_state: 'backlog',
	default_priority: 10,
	labels: []
};

function docketRoot(projectId?: string): string {
	if (projectId) {
		const project = getProject(projectId);
		if (project) return project.dir;
	}
	return process.env.DOCKET_DIR ?? process.cwd();
}

export function readConfig(projectId?: string): Config {
	const p = path.join(docketRoot(projectId), '.docket', 'config.json');
	if (!fs.existsSync(p)) {
		return defaultConfig;
	}
	const raw = JSON.parse(fs.readFileSync(p, 'utf8')) as Partial<Config>;
	return {
		states: raw.states ?? defaultConfig.states,
		default_state: raw.default_state ?? defaultConfig.default_state,
		default_priority: raw.default_priority ?? defaultConfig.default_priority,
		labels: raw.labels ?? defaultConfig.labels
	};
}

function parseTicketFile(content: string): Ticket | null {
	const parts = content.split('---\n');
	if (parts.length < 3) return null;
	const frontmatter = parts[1];
	const body = parts.slice(2).join('---\n').trim();

	const fmObj: Record<string, unknown> = {};
	for (const line of frontmatter.split('\n')) {
		const trimmed = line.trim();
		if (!trimmed || !trimmed.includes(':')) continue;
		const idx = trimmed.indexOf(':');
		const key = trimmed.slice(0, idx).trim();
		const rawValue = trimmed.slice(idx + 1).trim();
		if (rawValue.startsWith('[') && rawValue.endsWith(']')) {
			const vals = rawValue
				.slice(1, -1)
				.split(',')
				.map((v) => v.trim().replace(/^"(.*)"$/, '$1'))
				.filter(Boolean);
			fmObj[key] = vals;
			continue;
		}
		fmObj[key] = rawValue.replace(/^"(.*)"$/, '$1');
	}

	const titleMatch = body.match(/^#\s+[A-Z]+-\d+:\s+(.+)$/m) ?? body.match(/^#\s+(.+)$/m);
	const title = titleMatch?.[1]?.trim() ?? 'Untitled';
	const fm = fmObj as Record<string, unknown> & FrontmatterTicket;
	return {
		id: String(fm.id ?? ''),
		seq: Number(fm.seq ?? 0),
		state: String(fm.state ?? ''),
		priority: Number(fm.priority ?? 0),
		labels: Array.isArray(fm.labels) ? (fm.labels as string[]) : [],
		parent: fm.parent ? String(fm.parent) : undefined,
		title,
		created_at: String(fm.created_at ?? ''),
		updated_at: String(fm.updated_at ?? ''),
		body
	};
}

export function readTickets(projectId?: string): Ticket[] {
	const dir = path.join(docketRoot(projectId), '.docket', 'tickets');
	if (!fs.existsSync(dir)) {
		return [];
	}
	const out: Ticket[] = [];
	for (const file of fs.readdirSync(dir)) {
		if (!file.endsWith('.md')) continue;
		const full = path.join(dir, file);
		const stat = fs.statSync(full);
		if (!stat.isFile()) continue;
		const parsed = parseTicketFile(fs.readFileSync(full, 'utf8'));
		if (parsed) out.push(parsed);
	}
	out.sort((a, b) => a.priority - b.priority || a.seq - b.seq);
	return out;
}
