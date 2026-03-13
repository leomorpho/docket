import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { getProject } from '$lib/server/registry';
import path from 'node:path';
import fs from 'node:fs';

const execFileAsync = promisify(execFile);

type CreateBody = {
	title: string;
	desc?: string;
	state?: string;
	priority?: number;
	labels?: string[];
	parent?: string;
	projectId?: string;
};

export const POST: RequestHandler = async ({ request }) => {
	const body = (await request.json().catch(() => ({}))) as CreateBody;
	if (!body.title) {
		return json({ ok: false, error: 'title is required' }, { status: 400 });
	}

	const projectId = body.projectId;
	let dir = process.env.DOCKET_DIR ?? process.cwd();
	if (projectId) {
		const project = getProject(projectId);
		if (project) dir = project.dir;
	}

	const docketBin = process.env.DOCKET_BIN || (fs.existsSync(path.join(dir, 'docket')) ? path.join(dir, 'docket') : 'docket');
	const args = ['create', '--title', body.title, '--format', 'json'];
	if (body.desc) args.push('--desc', body.desc);
	if (body.state) args.push('--state', body.state);
	if (body.priority) args.push('--priority', String(body.priority));
	if (body.labels && body.labels.length > 0) args.push('--labels', body.labels.join(','));
	if (body.parent) args.push('--parent', body.parent);

	try {
		const { stdout } = await execFileAsync(docketBin, args, {
			cwd: dir,
			env: { ...process.env, DOCKET_DIR: dir }
		});
		const result = JSON.parse(stdout);
		return json({ ok: true, ticket: result });
	} catch (err: any) {
		return json({ ok: false, error: err.message }, { status: 500 });
	}
};
