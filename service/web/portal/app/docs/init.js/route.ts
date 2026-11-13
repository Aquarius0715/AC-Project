import { relayApiPage } from "@/lib/relay";

// GET /docs/init.js: the Core API's page, relayed from the gateway (IR791)
export const GET = () => relayApiPage("/docs/init.js");
