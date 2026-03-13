import type { RequestHandler } from './$types';
import { json } from '@sveltejs/kit';
import { getSecureModeStatus } from '$lib/server/docket-cli';

export const GET: RequestHandler = async ({ url }) => {
	const projectId = url.searchParams.get('projectId') ?? undefined;
	const status = await getSecureModeStatus(projectId);
	return json({ ok: true, secure: status });
};
