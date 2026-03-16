import fs from 'node:fs';
import path from 'node:path';
import type { AcceptanceCriterion, Comment, Config, PlanStep, Proof, Relation, Ticket } from '$lib/types';
import { getProject } from '$lib/server/registry';

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

type ParsedBody = {
	description: string;
	sections: Record<string, string>;
};

function parseBodySections(body: string): ParsedBody {
	const lines = body.split('\n');
	const titleLineIndex = lines.findIndex((line) => /^#\s+/.test(line));
	const contentStart = titleLineIndex >= 0 ? titleLineIndex + 1 : 0;
	let firstSectionIndex = lines.length;
	for (let i = contentStart; i < lines.length; i += 1) {
		if (/^##\s+/.test(lines[i])) {
			firstSectionIndex = i;
			break;
		}
	}

	const sections: Record<string, string> = {};
	let currentSectionKey: string | null = null;
	let currentSectionLines: string[] = [];
	for (let i = firstSectionIndex; i < lines.length; i += 1) {
		const line = lines[i];
		const headingMatch = line.match(/^##\s+(.+?)\s*$/);
		if (headingMatch) {
			if (currentSectionKey) {
				sections[currentSectionKey] = currentSectionLines.join('\n').trim();
			}
			currentSectionKey = headingMatch[1].trim().toLowerCase();
			currentSectionLines = [];
			continue;
		}
		if (currentSectionKey) {
			currentSectionLines.push(line);
		}
	}
	if (currentSectionKey) {
		sections[currentSectionKey] = currentSectionLines.join('\n').trim();
	}

	const fallbackDescription = lines.slice(contentStart, firstSectionIndex).join('\n').trim();
	const description = sections.description?.trim() || fallbackDescription;
	return { description, sections };
}

function parseAcceptanceCriteria(section: string | undefined): AcceptanceCriterion[] {
	if (!section) return [];
	const ac: AcceptanceCriterion[] = [];
	for (const line of section.split('\n')) {
		const m = line.trim().match(/^- \[(x| )\] (.*)$/);
		if (!m) continue;
		const done = m[1] === 'x';
		let desc = m[2].trim();
		let evidence: string | undefined;
		if (desc.includes(' — evidence: ')) {
			const parts = desc.split(' — evidence: ');
			desc = parts[0];
			evidence = parts[1];
		} else if (desc.includes(' : ')) {
			const parts = desc.split(' : ');
			desc = parts[0];
			evidence = parts[1];
		}
		ac.push({ done, description: desc, evidence });
	}
	return ac;
}

function parsePlan(section: string | undefined): PlanStep[] {
	if (!section) return [];
	const plan: PlanStep[] = [];
	for (const line of section.split('\n')) {
		const m = line.trim().match(/^\d+\. \[(.*?)\] (.*)$/);
		if (!m) continue;
		const status = m[1].trim();
		let desc = m[2].trim();
		let notes: string | undefined;
		if (desc.includes(' — ')) {
			const parts = desc.split(' — ');
			desc = parts[0];
			notes = parts[1];
		} else if (desc.includes(' : ')) {
			const parts = desc.split(' : ');
			desc = parts[0];
			notes = parts[1];
		}
		plan.push({ status, description: desc, notes });
	}
	return plan;
}

function parseComments(section: string | undefined): Comment[] {
	if (!section) return [];
	const comments: Comment[] = [];
	const headerPattern =
		/^(?:###\s*)?(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z)\s+—\s+(.+)$/;

	let currentAt: string | null = null;
	let currentAuthor: string | null = null;
	let currentBodyLines: string[] = [];

	const flush = () => {
		if (!currentAt || !currentAuthor) return;
		comments.push({
			at: currentAt,
			author: currentAuthor,
			body: currentBodyLines.join('\n').trim()
		});
		currentAt = null;
		currentAuthor = null;
		currentBodyLines = [];
	};

	for (const rawLine of section.split('\n')) {
		const line = rawLine.replace(/\r$/, '');
		const headerMatch = line.trim().match(headerPattern);
		if (headerMatch) {
			flush();
			currentAt = headerMatch[1];
			currentAuthor = headerMatch[2].trim();
			currentBodyLines = [];
			continue;
		}
		if (currentAt && currentAuthor) {
			currentBodyLines.push(line);
		}
	}
	flush();
	if (comments.length === 0 && section.trim()) {
		comments.push({
			at: '',
			author: 'unstructured',
			body: section.trim()
		});
	}
	return comments;
}

function buildFrontmatterRecord(
	fmObj: Record<string, unknown>
): Record<string, string | string[]> | undefined {
	const frontmatter: Record<string, string | string[]> = {};
	for (const [key, value] of Object.entries(fmObj)) {
		if (Array.isArray(value)) {
			const items = value
				.map((entry) => String(entry).trim())
				.filter(Boolean);
			if (items.length > 0) {
				frontmatter[key] = items;
			}
			continue;
		}
		if (value === undefined || value === null) continue;
		const normalized = String(value).trim();
		if (!normalized) continue;
		frontmatter[key] = normalized;
	}
	return Object.keys(frontmatter).length > 0 ? frontmatter : undefined;
}

function parseTicketFile(content: string): Ticket | null {
	const parts = content.split('---\n');
	if (parts.length < 3) return null;
	const frontmatter = parts[1];
	const body = parts.slice(2).join('---\n').trim();

	const fmObj = parseFrontmatter(frontmatter);
	const titleMatch = body.match(/^#\s+[A-Z]+-\d+:\s+(.+)$/m) ?? body.match(/^#\s+(.+)$/m);
	const title = titleMatch?.[1]?.trim() ?? 'Untitled';
	const fm = fmObj as Record<string, unknown>;
	const parsedBody = parseBodySections(body);
	const ac = parseAcceptanceCriteria(parsedBody.sections['acceptance criteria']);
	const plan = parsePlan(parsedBody.sections.plan);
	const comments = parseComments(parsedBody.sections.comments);
	const handoff = parsedBody.sections.handoff?.trim() || undefined;
	const description = parsedBody.description || undefined;
	const frontmatterRecord = buildFrontmatterRecord(fmObj);

	return {
		id: String(fm.id ?? ''),
		seq: Number(fm.seq ?? 0),
		state: String(fm.state ?? ''),
		priority: Number(fm.priority ?? 0),
		labels: toStringArray(fm.labels),
		blocked_by: toStringArray(fm.blocked_by),
		parent: fm.parent ? String(fm.parent) : undefined,
		children: toStringArray((fm as Record<string, unknown>).children),
		title,
		created_at: String(fm.created_at ?? ''),
		updated_at: String(fm.updated_at ?? ''),
		started_at: fm.started_at ? String(fm.started_at) : undefined,
		completed_at: fm.completed_at ? String(fm.completed_at) : undefined,
		created_by: fm.created_by ? String(fm.created_by) : undefined,
		write_hash: fm.write_hash ? String(fm.write_hash) : undefined,
		description,
		ac,
		plan,
		comments,
		handoff,
		frontmatter: frontmatterRecord,
		body
	};
}

function parseFrontmatter(frontmatter: string): Record<string, unknown> {
	const fmObj: Record<string, unknown> = {};
	let pendingListKey: string | null = null;
	for (const rawLine of frontmatter.split('\n')) {
		const line = rawLine.replace(/\r$/, '');
		const listItemMatch = line.match(/^\s*-\s+(.*)$/);
		if (listItemMatch && pendingListKey) {
			const list = Array.isArray(fmObj[pendingListKey]) ? (fmObj[pendingListKey] as unknown[]) : [];
			list.push(listItemMatch[1].trim().replace(/^"(.*)"$/, '$1'));
			fmObj[pendingListKey] = list;
			continue;
		}

		const trimmed = line.trim();
		if (!trimmed) continue;
		const keyMatch = line.match(/^([A-Za-z0-9_]+):\s*(.*)$/);
		if (!keyMatch) {
			pendingListKey = null;
			continue;
		}

		const key = keyMatch[1];
		const rawValue = keyMatch[2].trim();
		if (!rawValue) {
			pendingListKey = key;
			fmObj[key] = '';
			continue;
		}

		pendingListKey = null;
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
	return fmObj;
}

function toStringArray(value: unknown): string[] {
	if (Array.isArray(value)) {
		return value
			.map((entry) => String(entry).trim())
			.filter(Boolean);
	}
	if (typeof value === 'string' && value.trim()) {
		return [value.trim()];
	}
	return [];
}

function parseProofs(ticketID: string, projectId?: string): Proof[] {
	const metadataPath = path.join(docketRoot(projectId), '.docket', 'proofs', ticketID, 'metadata.json');
	if (!fs.existsSync(metadataPath)) return [];
	try {
		const raw = JSON.parse(fs.readFileSync(metadataPath, 'utf8'));
		if (!Array.isArray(raw)) return [];
		const proofs: Proof[] = [];
		for (const entry of raw) {
			const rec = entry as Record<string, unknown>;
			const file = (rec.file ?? {}) as Record<string, unknown>;
			const id = String(rec.id ?? '').trim();
			if (!id) continue;
			proofs.push({
				id,
				ticket_id: String(rec.ticket_id ?? ticketID),
				proof_title: String(rec.proof_title ?? ''),
				note: String(rec.note ?? ''),
				added_at: String(rec.added_at ?? ''),
				captured_at: rec.captured_at ? String(rec.captured_at) : undefined,
				actor: rec.actor ? String(rec.actor) : undefined,
				file: {
					path: String(file.path ?? ''),
					mime_type: String(file.mime_type ?? 'application/octet-stream'),
					size_bytes: Number(file.size_bytes ?? 0),
					sha256: String(file.sha256 ?? '')
				}
			});
		}
		return proofs.sort((a, b) => {
			if (a.added_at === b.added_at) return a.id.localeCompare(b.id);
			return b.added_at.localeCompare(a.added_at);
		});
	} catch {
		return [];
	}
}

function parseTicketSummaryFile(content: string): Ticket | null {
	const parts = content.split('---\n');
	if (parts.length < 3) return null;
	const frontmatter = parts[1];
	const body = parts.slice(2).join('---\n').trim();

	const fmObj = parseFrontmatter(frontmatter);
	const titleMatch = body.match(/^#\s+[A-Z]+-\d+:\s+(.+)$/m) ?? body.match(/^#\s+(.+)$/m);
	const title = titleMatch?.[1]?.trim() ?? 'Untitled';
	const fm = fmObj as Record<string, unknown>;
	const parsedBody = parseBodySections(body);
	const frontmatterRecord = buildFrontmatterRecord(fmObj);

	return {
		id: String(fm.id ?? ''),
		seq: Number(fm.seq ?? 0),
		state: String(fm.state ?? ''),
		priority: Number(fm.priority ?? 0),
		labels: toStringArray(fm.labels),
		blocked_by: toStringArray(fm.blocked_by),
		parent: fm.parent ? String(fm.parent) : undefined,
		children: toStringArray((fm as Record<string, unknown>).children),
		title,
		created_at: String(fm.created_at ?? ''),
		updated_at: String(fm.updated_at ?? ''),
		started_at: fm.started_at ? String(fm.started_at) : undefined,
		completed_at: fm.completed_at ? String(fm.completed_at) : undefined,
		created_by: fm.created_by ? String(fm.created_by) : undefined,
		write_hash: fm.write_hash ? String(fm.write_hash) : undefined,
		description: parsedBody.description || undefined,
		ac: [],
		plan: [],
		comments: [],
		handoff: undefined,
		frontmatter: frontmatterRecord,
		body: ''
	};
}

export function readTickets(projectId?: string): Ticket[] {
	const ticketsDir = path.join(docketRoot(projectId), '.docket', 'tickets');
	if (!fs.existsSync(ticketsDir)) {
		return [];
	}

	const entries = fs
		.readdirSync(ticketsDir)
		.filter((name) => name.endsWith('.md') && name.startsWith('TKT-'))
		.sort();

	const tickets: Ticket[] = [];
	for (const entry of entries) {
		const file = path.join(ticketsDir, entry);
		try {
			const parsed = parseTicketFile(fs.readFileSync(file, 'utf8'));
			if (parsed && parsed.id) {
				parsed.proofs = parseProofs(parsed.id, projectId);
				tickets.push(parsed);
			}
		} catch {
			// Ignore malformed ticket files in UI read path.
		}
	}

	return tickets.sort((a, b) => {
		if (a.priority !== b.priority) return a.priority - b.priority;
		if (a.seq !== b.seq) return a.seq - b.seq;
		return a.id.localeCompare(b.id);
	});
}

export function readTicketSummaries(projectId?: string): Ticket[] {
	const ticketsDir = path.join(docketRoot(projectId), '.docket', 'tickets');
	if (!fs.existsSync(ticketsDir)) {
		return [];
	}

	const entries = fs
		.readdirSync(ticketsDir)
		.filter((name) => name.endsWith('.md') && name.startsWith('TKT-'))
		.sort();

	const tickets: Ticket[] = [];
	for (const entry of entries) {
		const file = path.join(ticketsDir, entry);
		try {
			const parsed = parseTicketSummaryFile(fs.readFileSync(file, 'utf8'));
			if (parsed && parsed.id) {
				parsed.proofs = parseProofs(parsed.id, projectId);
				tickets.push(parsed);
			}
		} catch {
			// Ignore malformed ticket files in UI read path.
		}
	}

	return tickets.sort((a, b) => {
		if (a.priority !== b.priority) return a.priority - b.priority;
		if (a.seq !== b.seq) return a.seq - b.seq;
		return a.id.localeCompare(b.id);
	});
}

export function readTicket(id: string, projectId?: string): Ticket | null {
	if (!id) return null;
	const file = path.join(docketRoot(projectId), '.docket', 'tickets', `${id}.md`);
	if (!fs.existsSync(file)) return null;
	try {
		const parsed = parseTicketFile(fs.readFileSync(file, 'utf8'));
		if (!parsed || !parsed.id) return null;
		parsed.proofs = parseProofs(parsed.id, projectId);
		return parsed;
	} catch {
		return null;
	}
}

export function readTicketProofBlob(
	ticketID: string,
	proofID: string,
	projectId?: string
): { bytes: Buffer; mimeType: string } | null {
	const ticket = readTicket(ticketID, projectId);
	if (!ticket) return null;
	const proof = (ticket.proofs ?? []).find((entry) => entry.id === proofID);
	if (!proof) return null;

	const root = docketRoot(projectId);
	const proofPath = path.resolve(root, proof.file.path);
	const rootPath = path.resolve(root);
	if (!proofPath.startsWith(rootPath + path.sep) && proofPath !== rootPath) {
		return null;
	}
	if (!fs.existsSync(proofPath)) return null;
	try {
		return {
			bytes: fs.readFileSync(proofPath),
			mimeType: proof.file.mime_type || 'application/octet-stream'
		};
	} catch {
		return null;
	}
}

export function readRelations(projectId?: string): Relation[] {
	const p = path.join(docketRoot(projectId), '.docket', 'relations.json');
	if (!fs.existsSync(p)) {
		return [];
	}
	try {
		const raw = JSON.parse(fs.readFileSync(p, 'utf8'));
		return raw.relations ?? [];
	} catch {
		return [];
	}
}
