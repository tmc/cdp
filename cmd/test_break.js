const http = require('http');

// Start a dummy server to fetch from
const server = http.createServer((req, res) => {
    res.writeHead(200);
    res.end('ok');
}).listen(9999, () => {
    console.log('Server listening on 9999');
    runLoop();
});

async function runLoop() {
    let count = 0;
    setInterval(async () => {
        count++;
        console.log('Tick ' + count); // Line 14: Good for line breakpoint

        if (count % 5 === 0) {
            console.log('Fetching...');
            try {
                await fetch('http://localhost:9999/api/data');
            } catch (e) { console.error(e); }
        }
    }, 1000);
}
