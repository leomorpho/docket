import fs from 'node:fs';
import path from 'node:path';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { getProject } from '$lib/server/registry';

const execFileAsync = promisify(execFile);

const ticketIDRe = /^TKT-\d+$/;

export type TicketMutation =
	| { kind: 'state'; value: string }
	| { kind: 'title'; value: string }
	| { kind: 'desc'; value: string }
	| { kind: 'ac-complete'; value: string; evidence?: string }
	| { kind: 'comment'; value: string };

export type MutationResult = {
	ok: true;
	id: string;
	updatedFields: string[];
} | {
	ok: false;
	error: string;
};

function docketRoot(projectId?: string): string {
	if (projectId) {
		const project = getProject(projectId);
		if (project) return project.dir;
	}
	return process.env.DOCKET_DIR ?? process.cwd();
}

function resolveDocketBinary(root: string): string {
	if (process.env.DOCKET_BIN) {
		return process.env.DOCKET_BIN;
	}
	const localBinary = path.join(root, 'docket');
	if (fs.existsSync(localBinary)) {
		return localBinary;
	}
	return 'docket';
}

function safeErrorMessage(stdout: string, stderr: string, fallback: string): string {
	const raw = `${stderr}\n${stdout}`.trim();
	if (!raw) return fallback;
	const firstLine = raw.split('\n').map((line) => line.trim()).find(Boolean);
	if (!firstLine) return fallback;
	return firstLine.length > 220 ? `${firstLine.slice(0, 220)}...` : firstLine;
}

function validateMutation(id: string, mutation: TicketMutation, allowedStates: Set<string>): string | null {
	if (!ticketIDRe.test(id)) {
		return 'Invalid ticket ID.';
	}
	if (mutation.kind === 'state') {
		const next = mutation.value.trim();
		if (!next) return 'State is required.';
		if (!allowedStates.has(next)) return `State "${next}" is not valid.`;
	}
	if (mutation.kind === 'title' && !mutation.value.trim()) {
		return 'Title cannot be empty.';
	}
	return null;
}

export async function runTicketMutation(
	id: string,
	mutation: TicketMutation,
	allowedStates: Set<string>,
	projectId?: string
): Promise<MutationResult> {
	const validationError = validateMutation(id, mutation, allowedStates);
	if (validationError) {
		return { ok: false, error: validationError };
	}

	const root = docketRoot(projectId);
	const docketBin = resolveDocketBinary(root);
	let args: string[] = [];
	
	if (mutation.kind === 'ac-complete') {
		args = ['ac', 'complete', id, mutation.value, '--format', 'json'];
		if (mutation.evidence) {
			args.push('--evidence', mutation.evidence);
		}
	} else if (mutation.kind === 'comment') {
		args = ['comment', id, '--body', mutation.value, '--format', 'json'];
	} else {
		args = ['update', id, '--format', 'json'];
		if (mutation.kind === 'state') {
			args.push('--state', mutation.value.trim());
		}
		if (mutation.kind === 'title') {
			args.push('--title', mutation.value.trim());
		}
		if (mutation.kind === 'desc') {
			args.push('--desc', mutation.value);
		}
	}

	try {
		const { stdout, stderr } = await execFileAsync(docketBin, args, {
			cwd: root,
			timeout: 10_000,
			maxBuffer: 1024 * 1024,
			env: { ...process.env, DOCKET_DIR: root }
		});
		const parsed = JSON.parse(stdout) as { id?: string; updated_fields?: unknown };
		const updatedFields = Array.isArray(parsed.updated_fields)
			? parsed.updated_fields.filter((v): v is string => typeof v === 'string')
			: [];
		return {
			ok: true,
			id: typeof parsed.id === 'string' ? parsed.id : id,
			updatedFields
		};
	} catch (error) {
		const err = error as NodeJS.ErrnoException & { stdout?: string; stderr?: string };
		return {
			ok: false,
			error: safeErrorMessage(err.stdout ?? '', err.stderr ?? '', 'Failed to update ticket.')
		};
	}
}
