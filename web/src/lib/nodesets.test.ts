import { describe, expect, it } from 'vitest';
import { celString, fieldMessages, regexToCel } from './nodesets';

describe('regexToCel', () => {
	it.each([
		['name', '^web-', "node.name.matches('^web-')"],
		['os', 'linux|darwin', "node.os.matches('linux|darwin')"],
		['name', "it's", "node.name.matches('it\\'s')"],
		['address', '10\\.0\\.', "node.address.matches('10\\\\.0\\\\.')"]
	] as const)('%s %s', (field, pattern, want) => {
		expect(regexToCel(field, pattern)).toBe(want);
	});
});

describe('celString', () => {
	it('escapes a backslash before a quote so the quote stays escaped', () => {
		expect(celString("\\'")).toBe("'\\\\\\''");
	});
	it('keeps a newline out of the literal', () => {
		expect(celString('a\nb')).toBe("'a\\nb'");
	});
});

describe('fieldMessages', () => {
	const fb = () => 'fallback';
	it('lists every field message of a 422', () => {
		expect(fieldMessages({ errors: [{ message: 'a' }, { message: 'b' }] }, fb)).toEqual(['a', 'b']);
	});
	it('falls back when there are none', () => {
		expect(fieldMessages({ errors: [] }, fb)).toEqual(['fallback']);
		expect(fieldMessages(new Error('x'), fb)).toEqual(['fallback']);
	});
});
