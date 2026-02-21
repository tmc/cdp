(() => {
    for (const t of document.querySelectorAll('button[role="switch"]')) {
        let e = t.parentElement;
        for (let i = 0; i < 10 && e; i++, e = e.parentElement) {
            if (e.textContent?.includes('Grounding') && !e.textContent?.includes('Function')) {
                if (t.getAttribute('aria-checked') === 'true') {
                    t.click();
                    return 'Disabled Grounding';
                }
                return 'Grounding already off';
            }
        }
    }
    return 'Grounding not found';
})();
