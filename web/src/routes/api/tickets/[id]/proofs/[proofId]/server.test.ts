import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { afterEach, describe, expect, it } from 'vitest';
import { GET } from './+server';

const originalDocketDir = process.env.DOCKET_DIR;

afterEach(() => {
	if (originalDocketDir === undefined) {
		delete process.env.DOCKET_DIR;
	} else {
		process.env.DOCKET_DIR = originalDocketDir;
	}
});

describe('ticket proof blob route', () => {
	it('serves proof blob bytes for known proof ids', async () => {
		const repo = fs.mkdtempSync(path.join(os.tmpdir(), 'docket-proof-route-'));
		const ticketDir = path.join(repo, '.docket', 'tickets');
		const proofTicketDir = path.join(repo, '.docket', 'proofs', 'TKT-001');
		const byHashDir = path.join(repo, '.docket', 'proofs', 'by-hash');
		fs.mkdirSync(ticketDir, { recursive: true });
		fs.mkdirSync(proofTicketDir, { recursive: true });
		fs.mkdirSync(byHashDir, { recursive: true });

		fs.writeFileSync(
			path.join(ticketDir, 'TKT-001.md'),
			`---
id: TKT-001
seq: 1
state: todo
priority: 1
labels: [feature]
created_at: "2026-03-14T00:00:00Z"
updated_at: "2026-03-14T00:00:00Z"
---

# TKT-001: route test
`,
			'utf8'
		);

		const blobRel = '.docket/proofs/by-hash/abc123.png';
		const blobAbs = path.join(repo, blobRel);
		fs.writeFileSync(blobAbs, Buffer.from([0x89, 0x50, 0x4e, 0x47]));
		fs.writeFileSync(
			path.join(proofTicketDir, 'metadata.json'),
			JSON.stringify(
				[
					{
						id: 'PRF-1',
						ticket_id: 'TKT-001',
						proof_title: 'Before',
						note: 'baseline',
						added_at: '2026-03-16T19:00:00Z',
						file: {
							path: blobRel,
							mime_type: 'image/png',
							size_bytes: 4,
							sha256: 'abc123'
						}
					}
				],
				null,
				2
			),
			'utf8'
		);

		process.env.DOCKET_DIR = repo;
		const res = await GET({
			params: { id: 'TKT-001', proofId: 'PRF-1' },
			url: new URL('http://localhost/api/tickets/TKT-001/proofs/PRF-1')
		} as never);
		expect(res.status).toBe(200);
		expect(res.headers.get('content-type')).toBe('image/png');
		const bytes = new Uint8Array(await res.arrayBuffer());
		expect(bytes.length).toBe(4);
	});

	it('returns 404 for missing proof id', async () => {
		process.env.DOCKET_DIR = fs.mkdtempSync(path.join(os.tmpdir(), 'docket-proof-route-miss-'));
		const res = await GET({
			params: { id: 'TKT-001', proofId: 'missing' },
			url: new URL('http://localhost/api/tickets/TKT-001/proofs/missing')
		} as never);
		expect(res.status).toBe(404);
	});
});
