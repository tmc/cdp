// Minimal FC handler for testing
window.__fcHandler = {
    check: () => {
        const inp = document.querySelector('input[placeholder="Enter function response"]');
        if (!inp) return { pending: false };
        const text = document.body.innerText;
        const fns = ['get_current_time', 'search_web', 'read_file', 'write_file', 'execute_command'];
        let fn = null, args = null;
        for (const f of fns) {
            if (text.includes(f)) {
                fn = f;
                const m = text.substring(text.indexOf(f)).match(/\{[\s\S]*?\}/);
                if (m) try { args = JSON.parse(m[0]); } catch(e) {}
                break;
            }
        }
        return { pending: true, fn, args };
    },
    respond: (json) => {
        const inp = document.querySelector('input[placeholder="Enter function response"]');
        if (!inp) return { ok: false };
        inp.focus();
        Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(inp, typeof json === 'string' ? json : JSON.stringify(json));
        inp.dispatchEvent(new Event('input', { bubbles: true }));
        return { ok: true };
    },
    send: () => {
        const btn = document.querySelector('button[type="submit"]');
        if (!btn || btn.disabled) return { ok: false };
        btn.click();
        return { ok: true };
    },
    time: (tz) => {
        const now = new Date();
        return { time: now.toISOString(), timezone: tz || 'UTC', unix: Math.floor(now.getTime()/1000) };
    },
    auto: () => {
        const c = window.__fcHandler.check();
        if (!c.pending || !c.fn) return { handled: false };
        let result;
        if (c.fn === 'get_current_time') result = window.__fcHandler.time(c.args?.timezone);
        else result = { error: 'Unknown function: ' + c.fn };
        window.__fcHandler.respond(result);
        window.__fcHandler.send();
        return { handled: true, fn: c.fn, result };
    }
};
'FC Handler loaded';
