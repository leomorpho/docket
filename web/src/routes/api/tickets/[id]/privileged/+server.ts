import type { RequestHandler } from './$types';
import { json } from '@sveltejs/kit';
import { readConfig } from '$lib/server/docket';
import { runPrivilegedStateMutation } from '$lib/server/docket-cli';

type PrivilegedMutationBody = {
	state?: string;
	approvalTicket?: string;
	confirm?: boolean;
	projectId?: string;
};

export const POST: RequestHandler = async ({ params, request }) => {
	const body = (await request.json().catch(() => ({}))) as PrivilegedMutationBody;
	if (typeof body.state !== 'string' || typeof body.approvalTicket !== 'string' || typeof body.confirm !== 'boolean') {
		return json({ ok: false, error: 'Invalid privileged mutation payload.' }, { status: 400 });
	}

	const projectId = body.projectId;
	const allowedStates = new Set(Object.keys(readConfig(projectId).states));
	const result = await runPrivilegedStateMutation(
		params.id,
		body.state,
		allowedStates,
		body.approvalTicket,
		body.confirm,
		projectId
	);
	if (!result.ok) {
		return json(result, { status: 400 });
	}
	return json(result);
};
