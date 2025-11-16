// test_fetch.js
console.log('Starting test script...');

function makeRequest() {
  console.log('Making a fetch request...');
  fetch('https://api.github.com/users/tmc').then(res => {
    console.log('Fetch request completed.');
    return res.json();
  }).then(data => {
    console.log('GitHub user data received.');
  });
}

// Wait a moment before making the request to ensure the debugger can attach.
setTimeout(makeRequest, 2000);

// Keep the process running for a bit.
setTimeout(() => {
  console.log('Test script finished.');
}, 15000);
