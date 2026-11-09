// ESLint of the k6 scenarios: the web's shared rule set (../eslint.base.mjs). The scenarios run in k6's own JavaScript
// runtime, not in Node or a browser, against the stack of the compose project ac-load.
import { webConfig } from "../eslint.base.mjs";

const config = [
  ...webConfig(import.meta.dirname),
  {
    // eslint-plugin-import overflows the stack ("Maximum call stack size exceeded") while it follows the `k6/http`
    // default export through @types/k6's re-exports; the rule would only compare import names with the module's
    // exports, which TypeScript checks for these files already
    rules: { "import/no-named-as-default": "off", "import/no-named-as-default-member": "off" },
  },
  // Each of these is a deliberate case for a load script:
  // - the jitter of the think time (and nothing else) uses Math.random: it is no secret and no token
  // - the defaults name the stack's services inside the ac-load compose network, which has no TLS; a real address is
  //   given by the environment
  // - an object is read by a key this script wrote itself (a setting's name, a metric's name, a kind): no input from
  //   outside reaches it
  {
    rules: {
      "sonarjs/pseudo-random": "off",
      "sonarjs/no-clear-text-protocols": "off",
      "security/detect-object-injection": "off",
    },
  },
];

export default config;
