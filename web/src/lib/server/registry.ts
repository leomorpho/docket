import crypto from 'node:crypto';
import path from 'node:path';
import fs from 'node:fs';
import type { Project } from '$lib/types';

// Module-level in-memory registry. Survives across requests for the lifetime
// of the SvelteKit dev server process. Auto-seeds from DOCKET_DIR on startup.
const registry = new Map<string, Project>();

export function projectId(dir: string): string {
	return crypto.createHash('sha1').update(dir).digest('hex').slice(0, 8);
}

export function registerProject(dir: string): Project {
	const id = projectId(dir);
	const existing = registry.get(id);
	if (existing) return existing;
	const project: Project = {
		id,
		dir,
		name: path.basename(dir),
		registeredAt: new Date().toISOString()
	};
	registry.set(id, project);
	return project;
}

export function getProject(id: string): Project | undefined {
	return registry.get(id);
}

export function listProjects(): Project[] {
	return Array.from(registry.values()).sort((a, b) => a.name.localeCompare(b.name));
}

// Seed from DOCKET_DIR env var so the initial project is always registered
// even before the CLI registers it via POST /api/projects/register.
const seedDir = process.env.DOCKET_DIR ?? process.cwd();
if (fs.existsSync(path.join(seedDir, '.docket'))) {
	registerProject(seedDir);
}
