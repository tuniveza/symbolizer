#!/usr/bin/env bun
// __NAME__ — run with ./make.sh [args]
export function greet(name: string): string {
  return `hello, ${name}`;
}

if (import.meta.main) {
  const [name = "world"] = process.argv.slice(2);
  console.log(greet(name));
}
