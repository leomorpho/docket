import type { RequestHandler } from './$types';
import { json } from '@sveltejs/kit';
import { registerProject, listProjects } from '$lib/server/registry';

export const GET: RequestHandler = async () => {
	return json({ projects: listProjects() });
};

type RegisterBody = { dir?: string };

export const POST: RequestHandler = async ({ request }) => {
	const body = (await request.json().catch(() => ({}))) as RegisterBody;
	if (!body.dir || typeof body.dir !== 'string') {
		return json({ ok: false, error: 'dir is required' }, { status: 400 });
	}
	const project = registerProject(body.dir);
	return json({ ok: true, project });
};
