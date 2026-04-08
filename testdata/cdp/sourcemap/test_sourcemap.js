function greeter(person) {
    return "Hello, " + person;
}
var user = "Jane User";
function tick() {
    console.log("Tick Reloaded");
    setTimeout(tick, 1000);
}
console.log(greeter(user));
tick();
//# sourceMappingURL=test_sourcemap.js.mapconsole.log("Hot Reloaded Event");
