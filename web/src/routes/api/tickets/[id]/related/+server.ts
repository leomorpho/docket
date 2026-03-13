import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { getProject } from '$lib/server/registry';
import path from 'node:path';
import fs from 'node:fs';

const execFileAsync = promisify(execFile);

export const GET: RequestHandler = async ({ params, url }) => {
	const projectId = url.searchParams.get('projectId');
	let dir = process.env.DOCKET_DIR ?? process.cwd();
	if (projectId) {
		const project = getProject(projectId);
		if (project) dir = project.dir;
	}

	const docketBin = process.env.DOCKET_BIN || (fs.existsSync(path.join(dir, 'docket')) ? path.join(dir, 'docket') : 'docket');
	const args = ['related', params.id, '--format', 'json', '--limit', '5'];

	try {
		const { stdout } = await execFileAsync(docketBin, args, {
			cwd: dir,
			env: { ...process.env, DOCKET_DIR: dir }
		});
		const result = JSON.parse(stdout);
		return json({ ok: true, related: result });
	} catch (err: any) {
		return json({ ok: false, error: err.message }, { status: 500 });
	}
};
