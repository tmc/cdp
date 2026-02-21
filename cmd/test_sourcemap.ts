
function greeter(person: string) {
    return "Hello, " + person;
}

let user = "Jane User";

function tick() {
    console.log("Tick");
    setTimeout(tick, 1000);
}

console.log(greeter(user));
tick();
