import type { RequestHandler } from './$types';
import { json } from '@sveltejs/kit';
import { readConfig } from '$lib/server/docket';
import { runTicketMutation, type TicketMutation } from '$lib/server/docket-cli';

type MutationBody = {
	kind?: 'state' | 'title' | 'desc' | 'ac-complete' | 'comment';
	value?: string;
	evidence?: string;
	projectId?: string;
};

export const PATCH: RequestHandler = async ({ params, request }) => {
	const body = (await request.json().catch(() => ({}))) as MutationBody;
	if (!body.kind || typeof body.value !== 'string') {
		return json({ ok: false, error: 'Invalid mutation payload.' }, { status: 400 });
	}

	const projectId = body.projectId;
	const mutation: TicketMutation = {
		kind: body.kind,
		value: body.value,
		evidence: body.evidence
	} as TicketMutation;
	const allowedStates = new Set(Object.keys(readConfig(projectId).states));
	const result = await runTicketMutation(params.id, mutation, allowedStates, projectId);
	if (!result.ok) {
		return json(result, { status: 400 });
	}
	return json(result);
};
