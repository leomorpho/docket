import { afterEach, describe, expect, it } from 'vitest';
import { PATCH as patchTicket } from './[id]/+server';
import { POST as postPrivileged } from './[id]/privileged/+server';
import { __resetDocketExecRunnerForTests, __setDocketExecRunnerForTests } from '$lib/server/docket-cli';

afterEach(() => {
	__resetDocketExecRunnerForTests();
});

describe('ticket mutation routes', () => {
	it('rejects privileged state through generic mutation endpoint', async () => {
		const request = new Request('http://localhost/api/tickets/TKT-189', {
			method: 'PATCH',
			headers: { 'content-type': 'application/json' },
			body: JSON.stringify({ kind: 'state', value: 'done' })
		});
		const response = await patchTicket({ params: { id: 'TKT-189' }, request } as any);
		expect(response.status).toBe(400);
		const payload = await response.json();
		expect(payload.ok).toBe(false);
		expect(payload.error).toContain('privileged');
	});

	it('supports privileged mutation route when secure mode is active', async () => {
		__setDocketExecRunnerForTests(async (_bin, args) => {
			if (args[0] === 'secure' && args[1] === 'status') {
				return { stdout: 'Secure mode active (expires: 2026-03-13T23:48:05Z)\n', stderr: '' };
			}
			if (args[0] === 'update') {
				return { stdout: JSON.stringify({ id: 'TKT-189', updated_fields: ['state'] }), stderr: '' };
			}
			throw new Error(`unexpected command: ${args.join(' ')}`);
		});

		const request = new Request('http://localhost/api/tickets/TKT-189/privileged', {
			method: 'POST',
			headers: { 'content-type': 'application/json' },
			body: JSON.stringify({ state: 'done', approvalTicket: 'TKT-189', confirm: true })
		});
		const response = await postPrivileged({ params: { id: 'TKT-189' }, request } as any);
		expect(response.status).toBe(200);
		const payload = await response.json();
		expect(payload.ok).toBe(true);
		expect(payload.id).toBe('TKT-189');
	});
});
