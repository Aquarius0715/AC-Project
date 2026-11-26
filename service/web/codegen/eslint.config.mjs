// ESLint of the code generators run with Node (codegen/*.ts): the web's shared rule set (../eslint.base.mjs).
import { webConfig } from "../eslint.base.mjs";

const config = [
  ...webConfig(import.meta.dirname),
  {
    // the generator reads and writes the repository's own files at paths it builds from its own location, never a path
    // from outside the process
    rules: { "security/detect-non-literal-fs-filename": "off" },
  },
];

export default config;
