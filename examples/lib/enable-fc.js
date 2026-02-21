(() => {
    for (const t of document.querySelectorAll('button[role="switch"]')) {
        let e = t.parentElement;
        for (let i = 0; i < 10 && e; i++, e = e.parentElement) {
            if (e.textContent?.includes('Function calling') && !e.textContent?.includes('Structured')) {
                if (t.disabled) return 'FC toggle disabled';
                if (t.getAttribute('aria-checked') === 'true') return 'FC already on';
                t.click();
                return 'Enabled FC';
            }
        }
    }
    return 'FC toggle not found';
})();
