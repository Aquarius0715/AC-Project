import { describe, expect, it } from "vitest";
import { addressFor, hostnameOf, issuerHeaders, publicHosts, withHost } from "@ac/web/lib/publicOrigin";

const env = { PUBLIC_HOSTS: "app.example, alt.example ,Lab.Example" };

// IR651: a request that came by a listed host gets that host; any other Host header gets the canonical address
describe("the addresses a deployment answers on", () => {
  it("reads the listed hosts without blanks, ports or case", () => {
    expect([publicHosts(env), publicHosts({}), publicHosts({ PUBLIC_HOSTS: " , " })]).toStrictEqual([["app.example", "alt.example", "lab.example"], [], []]);
  });

  it("reads the host name of a Host header, and nothing from a header that is no plain host", () => {
    expect([hostnameOf("alt.example:3200"), hostnameOf("Lab.Example"), hostnameOf("a b"), hostnameOf("evil.example/path"), hostnameOf("[::1]:3000"), hostnameOf(""), hostnameOf(null)])
      .toStrictEqual(["alt.example", "lab.example", null, null, null, null, null]);
  });

  it("swaps the host of an address and keeps its scheme, port and path", () => {
    expect([withHost("https://app.example:8281/realms/ac", "alt.example"), withHost("https://app.example", "lab.example")]).toStrictEqual(["https://alt.example:8281/realms/ac", "https://lab.example"]);
  });

  it("answers the listed host the request came by, and the canonical address otherwise", () => {
    const base = "https://app.example:8281/realms/ac";
    expect([
      addressFor(base, "alt.example:3200", env), // listed
      addressFor(base, "app.example:3200", env), // the canonical host itself
      addressFor(base, "evil.example", env), // anyone can send a Host header: not listed
      addressFor(base, "alt.example", {}), // nothing listed: nothing changes
      addressFor(base, null, env),
      addressFor("https://app.example:3200/", "alt.example:3200", env),
    ]).toStrictEqual([
      "https://alt.example:8281/realms/ac",
      base,
      base,
      base,
      base,
      "https://alt.example:3200",
    ]);
  });
});

/** A token that names `iss` (unsigned: only its payload is read). */
const token = (iss: string) => `x.${new TextEncoder().encode(JSON.stringify({ iss })).toBase64({ alphabet: "base64url", omitPadding: true })}.y`;

// IR651: a call to Keycloak's internal address names the address the token's sign-in started on
describe("the issuer of a token", () => {
  it("is named to Keycloak for a token of a listed address, and for no other", () => {
    expect([
      issuerHeaders(token("https://alt.example:8281/realms/ac"), env),
      issuerHeaders(token("https://app.example:8281/realms/ac"), env),
      issuerHeaders(token("https://evil.example:8281/realms/ac"), env),
      issuerHeaders(token("https://alt.example:8281/realms/ac"), {}),
      issuerHeaders("not a token", env),
    ]).toStrictEqual([
      { "X-Forwarded-Host": "alt.example:8281", "X-Forwarded-Proto": "https" },
      { "X-Forwarded-Host": "app.example:8281", "X-Forwarded-Proto": "https" },
      {},
      {},
      {},
    ]);
  });
});
