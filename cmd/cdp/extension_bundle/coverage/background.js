// Minimal service worker — creates a CDP-visible target for debugging.
// Enables extension_console and extension_evaluate MCP tools.
chrome.runtime.onInstalled.addListener(() => {
  console.log('CDP Coverage Explorer installed');
});
