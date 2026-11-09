// k6 puts a console in the script's global scope; @types/k6 does not declare it, and the DOM library is not wanted here.
declare const console: { error(message: string): void };
