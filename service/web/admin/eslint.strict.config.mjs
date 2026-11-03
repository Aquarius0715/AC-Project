// The strict ESLint of the admin app, being brought to zero findings (the user's "全部入り", 2026-11-03): the web's shared
// rule set (../eslint.base.mjs). It replaces eslint.config.mjs once nothing is left.
import { webConfig } from "../eslint.base.mjs";

export default webConfig(import.meta.dirname);
