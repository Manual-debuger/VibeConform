import type { Greeting } from './contracts/greeting'

export function greet(name: string): string {
  return `Hello, ${name}!`
}

export function greetAll(greeting: Greeting): string {
  return greet(greeting.name)
}
