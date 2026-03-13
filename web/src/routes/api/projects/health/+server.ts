import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { execSync } from 'node:child_process';
import { getProject } from '$lib/server/registry';
import path from 'node:path';
import fs from 'node:fs';

export const GET: RequestHandler = async ({ url }) => {
	const projectId = url.searchParams.get('projectId');
	let dir = process.env.DOCKET_DIR ?? process.cwd();
	if (projectId) {
		const project = getProject(projectId);
		if (project) dir = project.dir;
	}

	let output: string;
	try {
		// Run docket check --doctor --format json
		output = execSync(`docket check --doctor --format json --repo ${dir}`, {
			encoding: 'utf8',
			env: { ...process.env, PATH: `${process.env.PATH}:${process.cwd()}` }
		});
	} catch (err: any) {
		if (err.stdout) {
			output = err.stdout.toString();
		} else {
			return json({ ok: false, error: err.message }, { status: 500 });
		}
	}

	try {
		const result = JSON.parse(output);
		
		// Map CLI findings to our Finding type
		const findings = (result.findings || []).map((f: any) => ({
			ticketId: f.TicketID,
			rule: f.Rule,
			message: f.Message,
			severity: f.Severity === 'error' ? 'error' : 'warning'
		}));

		// Also calculate some stats from the tickets on disk for more metrics
		const ticketsDir = path.join(dir, '.docket', 'tickets');
		const stateDist: Record<string, number> = {};
		const prioDist: Record<number, number> = {};
		const invalidSigs: string[] = [];
		const staleTickets: string[] = [];
		let totalCycleTime = 0;
		let completedCount = 0;

		if (fs.existsSync(ticketsDir)) {
			const files = fs.readdirSync(ticketsDir).filter((f) => f.endsWith('.md'));
			for (const file of findings) {
				if (file.message.includes('Direct Mutation Detected')) {
					invalidSigs.push(file.ticketId);
				}
			}

			// We need to read tickets to get timestamps
			// (Ideally we'd have a server-side cache for this)
			for (const file of files) {
				try {
					const content = fs.readFileSync(path.join(ticketsDir, file), 'utf8');
					const fmMatch = content.match(/---([\s\S]*?)---/);
					if (fmMatch) {
						const fm = fmMatch[1];
						const startedAt = fm.match(/started_at: "(.*?)"/);
						const completedAt = fm.match(/completed_at: "(.*?)"/);
						if (startedAt && completedAt) {
							const start = new Date(startedAt[1]);
							const end = new Date(completedAt[1]);
							const cycleTime = (end.getTime() - start.getTime()) / (1000 * 60 * 60 * 24);
							totalCycleTime += cycleTime;
							completedCount++;
						}
					}
				} catch {
					// Skip unparseable
				}
			}
		}

		return json({
			ok: true,
			health: {
				ticketCount: result.checked,
				invalidSignatures: invalidSigs,
				staleTickets: staleTickets,
				stateDistribution: stateDist,
				priorityDistribution: prioDist,
				findings: findings,
				avgCycleTime: completedCount > 0 ? totalCycleTime / completedCount : 0
			}
		});
	} catch (err: any) {
		return json({ ok: false, error: err.message }, { status: 500 });
	}
};
