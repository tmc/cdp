(() => {
    const btn = [...document.querySelectorAll('button')].find(b => b.textContent.trim() === 'Save');
    if (btn) {
        btn.click();
        return 'Saved';
    }
    return 'Save button not found';
})();
