import fs from 'node:fs';
import path from 'node:path';
import { describe, expect, it } from 'vitest';

describe('DetailSheet proof rendering contract', () => {
	const componentPath = path.resolve('src/lib/components/DetailSheet.svelte');
	const source = fs.readFileSync(componentPath, 'utf8');

	it('contains explicit proof evidence section and empty-state messaging', () => {
		expect(source).toContain('Proof Evidence');
		expect(source).toContain('No screenshot proofs attached yet.');
		expect(source).toContain('{#if proofs.length === 0}');
	});

	it('renders proof cards with title, note, timestamps, and preview/link affordances', () => {
		expect(source).toContain('entry.proof_title');
		expect(source).toContain('entry.note');
		expect(source).toContain('entry.added_at');
		expect(source).toContain('entry.captured_at');
		expect(source).toContain('Open full image');
		expect(source).toContain('proofPreviewErrors');
		expect(source).toContain('Preview failed to load. Use the open-image link below.');
	});
});
