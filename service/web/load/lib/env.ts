// The settings of a run, from the environment k6 is started with (deploy/load/run.sh): where the Core API gateway and
// the identity provider answer inside the ac-load network, the load client's secrets, and the account manifest.
function setting(name: string, fallback?: string): string {
  const value = __ENV[name] ?? fallback;
  if (value === undefined || value === "") throw new Error(`${name} is not set`);
  return value;
}

export const env = {
  gatewayUrl: setting("GATEWAY_URL", "http://gateway:8080"),
  tokenUrl: setting("TOKEN_URL", "http://keycloak:8080/realms/ac/protocol/openid-connect/token"),
  clientId: setting("LOAD_CLIENT_ID", "ac-load"),
  clientSecret: setting("LOAD_CLIENT_SECRET"),
  adminClientId: setting("ADMIN_CLIENT_ID", "ac-admin-web"),
  adminClientSecret: setting("ADMIN_CLIENT_SECRET"),
  manifest: setting("MANIFEST", "/out/manifest.json"),
  // the stress scenario reads only: more virtual users than accounts would share a membership's write limit
  writes: setting("WRITES", "1") !== "0",
};
