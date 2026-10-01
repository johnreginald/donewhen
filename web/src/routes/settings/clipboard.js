// Tiny clipboard helper shared by the token secret and MCP connection panels.
// Returns whether the copy actually worked, so the caller can fall back to
// "select and copy manually" instead of claiming success it didn't have.
export async function copyToClipboard(text) {
	try {
		await navigator.clipboard.writeText(text);
		return true;
	} catch {
		return false;
	}
}
