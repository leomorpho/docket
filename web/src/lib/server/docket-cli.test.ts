import { afterEach, describe, expect, it } from 'vitest';
import {
	__resetDocketExecRunnerForTests,
	__setDocketExecRunnerForTests,
	getSecureModeStatus,
	runPrivilegedStateMutation,
	runTicketMutation
} from '$lib/server/docket-cli';

const allowedStates = new Set(['backlog', 'todo', 'in-progress', 'in-review', 'done', 'archived']);

afterEach(() => {
	__resetDocketExecRunnerForTests();
});

describe('runTicketMutation', () => {
	it('rejects privileged state on the agent-safe path', async () => {
		const result = await runTicketMutation('TKT-189', { kind: 'state', value: 'done' }, allowedStates);
		expect(result.ok).toBe(false);
		if (!result.ok) {
			expect(result.error).toContain('privileged');
		}
	});
});

describe('runPrivilegedStateMutation', () => {
	it('requires explicit confirmation before attempting privileged mutation', async () => {
		const calls: string[] = [];
		__setDocketExecRunnerForTests(async (_bin, args) => {
			calls.push(args.join(' '));
			return { stdout: '', stderr: '' };
		});

		const result = await runPrivilegedStateMutation('TKT-189', 'done', allowedStates, 'TKT-189', false);
		expect(result.ok).toBe(false);
		expect(calls.length).toBe(0);
	});

	it('fails when secure mode is inactive', async () => {
		__setDocketExecRunnerForTests(async (_bin, args) => {
			if (args[0] === 'secure' && args[1] === 'status') {
				return { stdout: 'Secure mode inactive.\n', stderr: '' };
			}
			throw new Error('unexpected command');
		});

		const result = await runPrivilegedStateMutation('TKT-189', 'done', allowedStates, 'TKT-189', true);
		expect(result.ok).toBe(false);
		if (!result.ok) {
			expect(result.error).toContain('inactive');
		}
	});

	it('runs privileged transition through secure-aware update path', async () => {
		const calls: string[][] = [];
		__setDocketExecRunnerForTests(async (_bin, args) => {
			calls.push(args);
			if (args[0] === 'secure' && args[1] === 'status') {
				return { stdout: 'Secure mode active (expires: 2026-03-13T23:48:05Z)\n', stderr: '' };
			}
			if (args[0] === 'update') {
				return { stdout: JSON.stringify({ id: 'TKT-189', updated_fields: ['state'] }), stderr: '' };
			}
			throw new Error(`unexpected command: ${args.join(' ')}`);
		});

		const result = await runPrivilegedStateMutation('TKT-189', 'done', allowedStates, 'TKT-189', true);
		expect(result.ok).toBe(true);
		expect(calls.length).toBe(2);
		expect(calls[0]).toEqual(['secure', 'status']);
		expect(calls[1]).toEqual([
			'update',
			'TKT-189',
			'--state',
			'done',
			'--ticket',
			'TKT-189',
			'--yes',
			'--format',
			'json'
		]);
	});
});

describe('getSecureModeStatus', () => {
	it('parses active secure mode output', async () => {
		__setDocketExecRunnerForTests(async () => ({
			stdout: 'Secure mode active (expires: 2026-03-13T23:48:05Z)\n',
			stderr: ''
		}));
		const status = await getSecureModeStatus();
		expect(status.active).toBe(true);
		expect(status.expiresAt).toBe('2026-03-13T23:48:05Z');
	});
});
