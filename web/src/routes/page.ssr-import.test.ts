import { describe, expect, it } from 'vitest';

describe('SSR route imports', () => {
	it('can import +page.svelte without missing module errors', async () => {
		const mod = await import('./+page.svelte');
		expect(mod.default).toBeTruthy();
	}, 15000);
});
