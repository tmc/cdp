// AI Studio Function Call Handler
// Detects pending function calls, executes them, and injects responses
//
// Usage: Inject this script into AI Studio, then call:
//   window.__fcHandler.check()     - Check for pending function calls
//   window.__fcHandler.respond(json) - Manually inject a response
//   window.__fcHandler.auto()      - Auto-handle with built-in executors

(() => {
    'use strict';

    // Built-in function executors
    const executors = {
        get_current_time: (args) => {
            const tz = args.timezone || 'UTC';
            const now = new Date();
            // Format in ISO with timezone info
            const options = { timeZone: tz, hour12: false };
            const formatter = new Intl.DateTimeFormat('en-CA', {
                ...options,
                year: 'numeric', month: '2-digit', day: '2-digit',
                hour: '2-digit', minute: '2-digit', second: '2-digit'
            });
            const parts = formatter.formatToParts(now);
            const get = (type) => parts.find(p => p.type === type)?.value || '';
            const dateStr = `${get('year')}-${get('month')}-${get('day')}T${get('hour')}:${get('minute')}:${get('second')}`;
            return {
                time: dateStr,
                timezone: tz,
                unix_timestamp: Math.floor(now.getTime() / 1000)
            };
        },

        search_web: (args) => {
            // Placeholder - would need actual web search integration
            return {
                query: args.query,
                results: [
                    { title: "Search result 1", snippet: "This is a placeholder search result for: " + args.query },
                    { title: "Search result 2", snippet: "Another placeholder result" }
                ],
                note: "This is a simulated search result"
            };
        },

        read_file: (args) => {
            // Placeholder - would need file system access
            return {
                path: args.path,
                error: "File reading not available in browser context",
                note: "Would need native integration to read files"
            };
        },

        write_file: (args) => {
            // Placeholder - would need file system access
            return {
                path: args.path,
                error: "File writing not available in browser context",
                note: "Would need native integration to write files"
            };
        },

        execute_command: (args) => {
            // Placeholder - would need shell access
            return {
                command: args.command,
                error: "Command execution not available in browser context",
                note: "Would need native integration to execute commands"
            };
        }
    };

    // Helper to set input value properly for Angular
    function setInputValue(input, value) {
        input.focus();
        const setter = Object.getOwnPropertyDescriptor(
            input.tagName === 'TEXTAREA' ? window.HTMLTextAreaElement.prototype : window.HTMLInputElement.prototype,
            'value'
        ).set;
        setter.call(input, value);
        input.dispatchEvent(new Event('input', { bubbles: true }));
        input.dispatchEvent(new Event('change', { bubbles: true }));
    }

    // Check if there's a pending function call
    function checkForFunctionCall() {
        const responseInput = document.querySelector('input[placeholder="Enter function response"]');
        if (!responseInput) {
            return { pending: false };
        }

        // Find the function call details in the page
        const pageText = document.body.innerText;

        // Look for function name patterns
        const functionNames = ['get_current_time', 'search_web', 'read_file', 'write_file', 'execute_command'];
        let detectedFunction = null;
        let detectedArgs = null;

        for (const fname of functionNames) {
            if (pageText.includes(fname)) {
                detectedFunction = fname;

                // Try to find the JSON args - look for JSON block after function name
                const fnIndex = pageText.indexOf(fname);
                const afterFn = pageText.substring(fnIndex);
                const jsonMatch = afterFn.match(/\{[\s\S]*?\}/);
                if (jsonMatch) {
                    try {
                        detectedArgs = JSON.parse(jsonMatch[0]);
                    } catch (e) {
                        // Try cleaning up the JSON
                        const cleaned = jsonMatch[0].replace(/[\n\r]/g, '').replace(/\s+/g, ' ');
                        try {
                            detectedArgs = JSON.parse(cleaned);
                        } catch (e2) {
                            detectedArgs = { raw: jsonMatch[0] };
                        }
                    }
                }
                break;
            }
        }

        return {
            pending: true,
            functionName: detectedFunction,
            args: detectedArgs,
            responseInput: responseInput
        };
    }

    // Inject a function response
    function injectResponse(responseJson) {
        const responseInput = document.querySelector('input[placeholder="Enter function response"]');
        if (!responseInput) {
            return { success: false, error: 'No function response input found' };
        }

        const responseStr = typeof responseJson === 'string' ? responseJson : JSON.stringify(responseJson);
        setInputValue(responseInput, responseStr);

        return { success: true, value: responseStr };
    }

    // Click the send button
    function clickSend() {
        const sendBtn = document.querySelector('button[type="submit"]');
        if (!sendBtn) {
            return { success: false, error: 'No send button found' };
        }

        if (sendBtn.classList.contains('invalid') || sendBtn.disabled) {
            return { success: false, error: 'Send button is disabled', className: sendBtn.className };
        }

        sendBtn.click();
        return { success: true };
    }

    // Auto-handle: detect, execute, respond
    function autoHandle() {
        const check = checkForFunctionCall();
        if (!check.pending) {
            return { handled: false, reason: 'No pending function call' };
        }

        if (!check.functionName) {
            return { handled: false, reason: 'Could not detect function name', check };
        }

        const executor = executors[check.functionName];
        if (!executor) {
            return { handled: false, reason: 'No executor for function: ' + check.functionName, check };
        }

        // Execute the function
        let result;
        try {
            result = executor(check.args || {});
        } catch (e) {
            result = { error: e.message };
        }

        // Inject the response
        const injectResult = injectResponse(result);
        if (!injectResult.success) {
            return { handled: false, reason: 'Failed to inject response', injectResult, check };
        }

        // Click send
        const sendResult = clickSend();

        return {
            handled: sendResult.success,
            functionName: check.functionName,
            args: check.args,
            result: result,
            sendResult: sendResult
        };
    }

    // Set up polling for auto-detection
    function startAutoMode(intervalMs = 1000) {
        if (window.__fcAutoInterval) {
            clearInterval(window.__fcAutoInterval);
        }

        window.__fcAutoInterval = setInterval(() => {
            const result = autoHandle();
            if (result.handled) {
                console.log('[FC-Handler] Auto-handled:', result);
            }
        }, intervalMs);

        console.log('[FC-Handler] Auto mode started, polling every ' + intervalMs + 'ms');
        return { started: true, interval: intervalMs };
    }

    // Stop auto mode
    function stopAutoMode() {
        if (window.__fcAutoInterval) {
            clearInterval(window.__fcAutoInterval);
            window.__fcAutoInterval = null;
            console.log('[FC-Handler] Auto mode stopped');
            return { stopped: true };
        }
        return { stopped: false, reason: 'Auto mode was not running' };
    }

    // Register a custom executor
    function registerExecutor(name, fn) {
        executors[name] = fn;
        console.log('[FC-Handler] Registered executor:', name);
        return { registered: true, name };
    }

    // Export the handler
    window.__fcHandler = {
        check: checkForFunctionCall,
        respond: injectResponse,
        send: clickSend,
        auto: autoHandle,
        startAuto: startAutoMode,
        stopAuto: stopAutoMode,
        register: registerExecutor,
        executors: executors
    };

    console.log('[FC-Handler] Loaded. Use window.__fcHandler.check() to detect function calls.');
    return { loaded: true, methods: Object.keys(window.__fcHandler) };
})();
