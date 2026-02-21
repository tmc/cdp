(() => {
    for (const b of document.querySelectorAll('button')) {
        if (b.textContent.trim() === 'Edit') {
            let e = b.parentElement;
            for (let i = 0; i < 10 && e; i++, e = e.parentElement) {
                if (e.textContent?.includes('Function calling') && !e.textContent?.includes('Structured')) {
                    if (!b.disabled) {
                        b.click();
                        return 'Opened dialog';
                    }
                    return 'Edit button disabled';
                }
            }
        }
    }
    return 'Edit button not found';
})();
