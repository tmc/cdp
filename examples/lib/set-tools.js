(() => {
    const tools = [
        {
            name: "get_current_time",
            description: "Gets the current date and time",
            parameters: {
                type: "object",
                properties: {
                    timezone: { type: "string", description: "Timezone like America/Los_Angeles" }
                }
            }
        },
        {
            name: "search_web",
            description: "Searches the web for information",
            parameters: {
                type: "object",
                properties: {
                    query: { type: "string", description: "The search query" }
                },
                required: ["query"]
            }
        },
        {
            name: "read_file",
            description: "Reads contents of a file",
            parameters: {
                type: "object",
                properties: {
                    path: { type: "string", description: "File path to read" }
                },
                required: ["path"]
            }
        },
        {
            name: "write_file",
            description: "Writes content to a file",
            parameters: {
                type: "object",
                properties: {
                    path: { type: "string", description: "File path to write" },
                    content: { type: "string", description: "Content to write" }
                },
                required: ["path", "content"]
            }
        },
        {
            name: "execute_command",
            description: "Executes a shell command",
            parameters: {
                type: "object",
                properties: {
                    command: { type: "string", description: "Command to execute" }
                },
                required: ["command"]
            }
        }
    ];

    const dialog = document.querySelector('[role="dialog"], .cdk-overlay-pane');
    if (!dialog) return 'No dialog found';

    const textarea = dialog.querySelector('textarea');
    if (!textarea) return 'No textarea in dialog';

    textarea.focus();
    textarea.select();
    document.execCommand('selectAll', false, null);
    document.execCommand('insertText', false, JSON.stringify(tools, null, 2));

    return 'Set ' + tools.length + ' tool definitions';
})();
