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

export type SecureStatus = {
	active: boolean;
	expiresAt?: string;
	error?: string;
};

type ExecResult = {
	stdout: string;
	stderr: string;
};

type ExecRunner = (docketBin: string, args: string[], opts: { cwd: string; env: NodeJS.ProcessEnv }) => Promise<ExecResult>;

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

function isPrivilegedState(next: string): boolean {
	return next === 'done' || next === 'archived';
}

function validateMutation(id: string, mutation: TicketMutation, allowedStates: Set<string>): string | null {
	if (!ticketIDRe.test(id)) {
		return 'Invalid ticket ID.';
	}
	if (mutation.kind === 'state') {
		const next = mutation.value.trim();
		if (!next) return 'State is required.';
		if (!allowedStates.has(next)) return `State "${next}" is not valid.`;
		if (isPrivilegedState(next)) {
			return `State "${next}" is privileged. Use the secure/admin workflow path instead.`;
		}
	}
	if (mutation.kind === 'title' && !mutation.value.trim()) {
		return 'Title cannot be empty.';
	}
	return null;
}

let execRunner: ExecRunner = async (docketBin, args, opts) =>
	execFileAsync(docketBin, args, {
		cwd: opts.cwd,
		timeout: 10_000,
		maxBuffer: 1024 * 1024,
		env: opts.env
	});

async function runDocket(root: string, args: string[]): Promise<ExecResult> {
	const docketBin = resolveDocketBinary(root);
	return execRunner(docketBin, args, { cwd: root, env: { ...process.env, DOCKET_DIR: root } });
}

export function __setDocketExecRunnerForTests(next: ExecRunner): void {
	execRunner = next;
}

export function __resetDocketExecRunnerForTests(): void {
	execRunner = async (docketBin, args, opts) =>
		execFileAsync(docketBin, args, {
			cwd: opts.cwd,
			timeout: 10_000,
			maxBuffer: 1024 * 1024,
			env: opts.env
		});
}

export async function getSecureModeStatus(projectId?: string): Promise<SecureStatus> {
	const root = docketRoot(projectId);
	try {
		const { stdout } = await runDocket(root, ['secure', 'status']);
		const out = stdout.trim();
		if (/inactive/i.test(out)) {
			return { active: false };
		}
		const match = out.match(/\(expires:\s*([^)]+)\)/i);
		return {
			active: /active/i.test(out),
			expiresAt: match?.[1]?.trim()
		};
	} catch (error) {
		const err = error as NodeJS.ErrnoException & { stdout?: string; stderr?: string };
		const msg = safeErrorMessage(err.stdout ?? '', err.stderr ?? '', 'Failed to query secure mode state.');
		if (/inactive/i.test(msg)) {
			return { active: false };
		}
		return { active: false, error: msg };
	}
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
		const { stdout } = await runDocket(root, args);
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

export async function runPrivilegedStateMutation(
	id: string,
	state: string,
	allowedStates: Set<string>,
	approvalTicket: string,
	approved: boolean,
	projectId?: string
): Promise<MutationResult> {
	if (!ticketIDRe.test(id)) {
		return { ok: false, error: 'Invalid ticket ID.' };
	}
	const next = state.trim();
	if (!next) {
		return { ok: false, error: 'State is required.' };
	}
	if (!allowedStates.has(next)) {
		return { ok: false, error: `State "${next}" is not valid.` };
	}
	if (!isPrivilegedState(next)) {
		return { ok: false, error: `State "${next}" is not privileged.` };
	}
	if (!ticketIDRe.test(approvalTicket)) {
		return { ok: false, error: 'Invalid approval ticket ID.' };
	}
	if (!approved) {
		return { ok: false, error: 'Privileged transition requires explicit confirmation.' };
	}

	const secure = await getSecureModeStatus(projectId);
	if (!secure.active) {
		return { ok: false, error: secure.error ?? 'Secure mode is inactive. Unlock secure mode before privileged changes.' };
	}

	const root = docketRoot(projectId);
	try {
		const { stdout } = await runDocket(root, [
			'update',
			id,
			'--state',
			next,
			'--ticket',
			approvalTicket,
			'--yes',
			'--format',
			'json'
		]);
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
			error: safeErrorMessage(err.stdout ?? '', err.stderr ?? '', 'Failed to run privileged mutation.')
		};
	}
}
