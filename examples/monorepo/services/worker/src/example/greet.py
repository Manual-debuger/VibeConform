from example.contracts.greeting import Greeting


def greet(name: str) -> str:
    return f"Hello, {name}!"


def greet_all(greeting: Greeting) -> str:
    return greet(greeting.name)
