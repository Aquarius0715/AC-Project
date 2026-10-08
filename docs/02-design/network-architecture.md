---
document_id: DD-NETWORK
version: 0.30.0
status: draft
owner: design-agent
consumers: [implementation-agent, review-agent, operations]
scope: production-target-logical
---

# Network Architecture (production target, AWS)

## 1. Purpose and scope

This document defines the network zones, exposed endpoints, traffic flows, and protection for the production platform described in the [backend architecture](backend-architecture.md). It started cloud-agnostic (IR116, DEC-67) and is now mapped to **AWS** (§2a; IR117, DEC-68). It is `PROPOSED`. Phase 1A runs entirely in the browser with the mock adapter and needs only static web hosting.

Principles:

1. Only the edge is reachable from the internet; application and data zones have no public addresses.
2. Field devices connect **outbound only**; no inbound port is opened at customer sites.
3. Every connection is encrypted (TLS 1.2 or later, TLS 1.3 preferred); device and internal service connections use mutual TLS where supported.
4. Outbound traffic to the internet leaves through controlled egress with an allowlist of provider names.
5. Two availability zones in the primary region; a second region for disaster recovery.

## 2. Zones

| Zone | Contains | Reachable from | Internet access |
|---|---|---|---|
| Z0 Internet | Browsers (office, home, mobile networks), field sites, external providers | — | — |
| Z1 Edge | DNS, CDN, WAF, DDoS protection, public load balancer, MQTT broker endpoint, webhook endpoint | Z0 on published ports only | Public |
| Z2 Web | Web/BFF (Next.js) containers | Z1 load balancer | Egress via Z5 only |
| Z3 Application | Core API, IoT gateway service, telemetry processor, automation engine, scheduler, notification and import / export workers, webhook receiver, outbox relay | Z2 (Core API only), Z1 (webhook receiver, MQTT bridge), Z6 | Egress via Z5 only |
| Z4 Data | Relational database (including partitioned telemetry tables), cache, message broker, object storage private endpoint, secret store endpoint | Z3 (and Z2 for the session cache) | None |
| Z5 Egress | NAT and egress proxy with FQDN allowlist | Z2, Z3 | Outbound to allowlisted providers |
| Z6 Management | CI/CD runners, operations access gateway (single sign-on, multi-factor), monitoring collectors | Operators through the access gateway | Controlled |
| Z7 Field sites | AC indoor units, IoT modules or site gateways, site uplink (Wi-Fi / LAN or cellular) | — (outbound only) | Outbound to Z1 MQTT endpoint and NTP |

## 2a. AWS mapping

| Zone | AWS implementation (PROPOSED) |
|---|---|
| Accounts and VPCs | AWS Organizations with separate dev, staging, prod workload accounts plus shared network and security / log archive accounts. One VPC per workload account per region (prod ap-southeast-5 `10.20.0.0/16`, DR ap-southeast-1 `10.21.0.0/16`, non-overlapping ranges for dev and staging), two availability zones |
| Z1 Edge | Route 53, CloudFront + AWS WAF + Shield Standard, public subnets with the Application Load Balancer (accepts only the CloudFront origin-facing prefix list and a secret origin header); AWS IoT Core configurable domain for `mqtt.<domain>`; S3 for `files.<domain>` via CloudFront signed requests or pre-signed URLs |
| Z2 Web | Private subnets (`/22` per AZ) with ECS Fargate tasks for the Next.js BFF; security group allows 443 / app port only from the ALB security group |
| Z3 Application | Private subnets (`/21` per AZ) with ECS Fargate services for the Core API and workers; Core API security group allows only the BFF security group; workers have no inbound rules |
| Z4 Data | Isolated subnets (`/23` per AZ, no route to the internet) for Aurora PostgreSQL, ElastiCache; VPC endpoints (interface or gateway) for S3, SQS, SNS, Kinesis, Secrets Manager, KMS, ECR, CloudWatch Logs, STS, IoT Core data plane |
| Z5 Egress | NAT gateways (one per AZ) behind AWS Network Firewall with a stateful FQDN / TLS SNI allowlist (Stripe API, Cognito, SES where not on endpoints, WhatsApp provider, weather source) |
| Z6 Management | AWS IAM Identity Center (SSO + MFA), AWS Systems Manager Session Manager for any task / host access (no bastion, no SSH), CI/CD via GitHub Actions OIDC or CodePipeline, CloudWatch / X-Ray collectors |
| Monitoring | VPC Flow Logs, WAF logs, ALB logs, Route 53 Resolver query logs, Network Firewall alert logs to the log archive account |

## 3. Public endpoints

| Name (example) | Purpose | Port | Behind |
|---|---|---|---|
| `app.<domain>` | Client, Partner and Technician apps (/customer, /partner, /technician) and shared routes, BFF routes `/bff/*`, SSE stream; `/admin` returns the shared Page unavailable view | 443 (HTTPS; port 80 redirects only) | CloudFront + WAF → ALB → Z2 |
| `admin.<domain>` | HQ admin app (/admin) and shared routes for HQ users | 443 | CloudFront + WAF **IP set: company network only** (office egress addresses and the company VPN) → ALB → Z2 (same Next.js service, host-based routing) |
| `hooks.<domain>` | Payment and messaging provider callbacks | 443 | WAF (provider IP allowlist where published) → Z3 webhook receiver |
| `mqtt.<domain>` | Device connections | 8883 (MQTT over TLS, mutual TLS); 443 as fallback (MQTT over secure WebSocket) for restrictive networks | Managed MQTT endpoint → Z3 IoT gateway |
| `files.<domain>` | Pre-signed upload and download of photos, attachments, reports | 443 | Object storage public endpoint, signed URLs only, short expiry |

The Core API, databases, broker, and management interfaces have no public names. HQ access is limited to the company network (DEC-68): requests to `admin.<domain>` from other addresses are blocked by WAF; HQ staff working remotely connect through the company VPN first. The BFF issues admin-role sessions only on `admin.<domain>` and the Core API rejects admin-role operations that do not carry that session mark (backend architecture §6). Changes to the IP set need a change request and are audited.

## 4. Traffic flows

| ID | From | To | Protocol / port | Authentication | Notes |
|---|---|---|---|---|---|
| F01 | Browser (Z0) | Edge → Web/BFF (Z2) | HTTPS 443 | Session cookie (httpOnly, Secure) | HSTS, CSP, WAF rules, rate limits per IP and per session |
| F01a | HQ browser on the company network or VPN | `admin.<domain>` → Web/BFF (Z2) | HTTPS 443 | WAF IP set + Cognito session + MFA | Any other source address is blocked at the edge |
| F02 | Browser (Z0) | Identity provider | HTTPS 443 | OIDC redirect | Sign-in, reset, two-step verification pages |
| F03 | Web/BFF (Z2) | Identity provider | HTTPS 443 via Z5 | Client credentials (private-key JWT) | Token exchange, logout |
| F04 | Web/BFF (Z2) | Core API (Z3) | HTTPS (internal), mutual TLS | Service token + user context | `/v1/ops/<operation>`; no other Z2 → Z3 path |
| F05 | Web/BFF (Z2) | Cache (Z4) | TLS 6379-class port | Service identity | Sessions only |
| F06 | Core API and workers (Z3) | Relational database (Z4) | TLS 5432-class port | Per-service database roles | Row-level security by tenant |
| F07 | Z3 services | Message broker, Kinesis, cache (Z4 endpoints) | TLS | Service identity | Private endpoints only |
| F08 | Z3 services | Object storage (Z4 private endpoint) | HTTPS 443 | Service identity | Issues pre-signed URLs for F12 |
| F09 | IoT module (Z7) | MQTT endpoint (Z1) | MQTT over TLS 8883 (fallback WSS 443) | Device X.509 certificate | Outbound only; keep-alive ≤ 60 seconds; topic access limited to its own deviceId |
| F10 | MQTT endpoint (Z1) | IoT gateway service (Z3) | Internal bridge / subscription | Service identity | Commands down, telemetry and acknowledgements up |
| F11 | Stripe / messaging provider | Webhook receiver (Z3) via Z1 (`hooks.<domain>/stripe`, `/whatsapp`, `/ses`) | HTTPS 443 | Stripe-Signature (5-minute tolerance) / provider signature | Idempotent by provider event ID; WAF allows Stripe's published webhook IP ranges on `/stripe` |
| F12 | Browser (Z0) | Object storage (`files.<domain>`) | HTTPS 443 | Pre-signed URL (≤ 10 minutes) | Upload size and type limits; scanned before visible |
| F13 | Notification worker (Z3) | E-mail and WhatsApp providers | HTTPS 443 via Z5 | API credentials | FQDN allowlist |
| F14 | Automation engine (Z3) | Weather source | HTTPS 443 via Z5 | API key | Polling |
| F15 | Webhook receiver / billing (Z3) | Stripe API (`api.stripe.com`) | HTTPS 443 via Z5 | Restricted API key | PaymentIntent retrieval before confirming a payment; Checkout Session creation |
| F16 | Operators (Z0) | Access gateway (Z6) → Z3 / Z4 | HTTPS 443 | Single sign-on + multi-factor, just-in-time roles | No SSH or database port open to the internet; sessions recorded |
| F17 | CI/CD (Z6) | Container registry, Z2 / Z3 deploy APIs | HTTPS 443 | Workload identity | Signed images only |
| F18 | All services | Monitoring collectors (Z6) | HTTPS / OTLP | Service identity | Logs, metrics, traces |
| F19 | IoT module (Z7) | Time source | NTP 123 (or the module's trusted time service) | — | Correct observedAt and certificate validation |

Any flow not in this table is denied.

## 5. Field site connectivity

| Topic | Design |
|---|---|
| Topology | AC indoor unit ↔ IoT module (local serial or vendor bus, OPEN-BE-02) ↔ site uplink ↔ internet ↔ `mqtt.<domain>`. A site gateway may serve several ACs on one site. |
| Uplink options | Home or office Wi-Fi / LAN (WPA2 or later, DHCP, outbound 8883 or 443), or a cellular modem (LTE-M / 4G) for sites without a usable network. |
| Inbound | None. No port forwarding or public IP is required at the site. |
| Traffic volume (estimate) | About 1 telemetry batch per minute per AC (≈ 0.5–1 KB) plus heartbeats and rare commands: roughly 20–45 MB per AC per month including TLS and MQTT overhead. |
| Offline | The module buffers telemetry and uploads it with original observedAt after reconnecting; reconnects use exponential backoff with jitter to avoid reconnect storms. |
| Site changes | A changed Wi-Fi password or router does not change the device identity; the technician re-provisions only the uplink. |

## 5a. Customer office firewall requirements

Give these to customer IT before installation (office sites; homes normally need nothing):

| Item | Requirement |
|---|---|
| Outbound MQTT | Allow TCP 8883 to `mqtt.<domain>` (AWS IoT Core custom endpoint). Rules must use the name, not IP addresses (the endpoint has no fixed IPs) |
| Fallback | If 8883 cannot be opened: allow TCP 443 to `mqtt.<domain>` (MQTT over secure WebSocket); an HTTP CONNECT proxy is supported for this mode |
| TLS inspection | Exclude `mqtt.<domain>` from TLS inspection / interception; mutual TLS with the device certificate fails if the traffic is re-signed |
| DNS | The module uses the site DHCP DNS resolver (UDP / TCP 53) and must be able to resolve `mqtt.<domain>` and the time source |
| Time | Allow UDP 123 to the time source (or the module's documented time service); certificate checks need correct time |
| Inbound | None. No port forwarding, public IP, or VPN to the site is required |
| Addressing | DHCP is enough; a DHCP reservation per module is recommended for the customer's own monitoring |
| Network placement | Put modules on an IoT / guest VLAN or SSID isolated from office PCs; client isolation on that SSID is fine (modules do not talk to each other) |
| Wi-Fi | 2.4 GHz WPA2-Personal or WPA2/WPA3 mixed (802.1X enterprise Wi-Fi only if the module supports it, OPEN-BE-02); signal at least −70 dBm at the indoor unit |
| Bandwidth | About 20–45 MB per AC per month; bursts during firmware updates (firmware size per model) |
| Captive portals | Not supported; use a network without a captive portal or the cellular option |

## 6. Edge protection and limits

- WAF with managed rule sets (injection, cross-site scripting, protocol violations) and project rules: block methods other than GET / POST on `app`, limit request body size, block known bad bots.
- Rate limits (PROPOSED): sign-in and password reset 10 per minute per IP and per account; writes 60 per minute per session; webhook endpoint per provider; MQTT connect attempts per device certificate.
- DDoS protection at the edge for `app`, `hooks`, and `mqtt`.
- Security headers on `app`: HSTS, Content-Security-Policy (self + identity provider + payment page redirect), X-Content-Type-Options, Referrer-Policy, frame-ancestors none.

## 7. Certificates and keys

| Item | Design |
|---|---|
| Public TLS certificates | Managed certificates for `app`, `hooks`, `mqtt`, `files`; automatic renewal |
| Internal mutual TLS | Private certificate authority or service mesh identities; short-lived certificates |
| Device certificates | Separate device certificate authority; one certificate per IoT module issued at provisioning; revoked on unbind, replacement, or tamper; rotation before expiry over MQTT |
| Data encryption keys | Managed key service; separate keys per environment; rotation yearly or on incident |

## 8. Availability and disaster recovery

- Primary region ap-southeast-5 (Malaysia) with two availability zones: load balancer, Web/BFF, Core API, workers, and the MQTT endpoint span both zones; the database has a synchronous standby in the second zone.
- DR region ap-southeast-1 (Singapore): Aurora cross-region snapshot copy or Aurora Global Database (if offered between the two regions), object storage replication, infrastructure as code ready to deploy; DNS failover with low TTL for `app`, `hooks`, and `mqtt`. Targets RPO 15 minutes and RTO 4 hours (backend architecture §13).
- Devices reconnect to the same name after failover; buffered telemetry covers the gap.

## 9. Network monitoring

- Flow logs for Z2–Z5, WAF and load balancer logs, DNS query logs for egress, MQTT connection and authorization failure metrics.
- Alerts: spike in denied flows or WAF blocks, webhook signature failures, MQTT authorization failures per certificate, egress to non-allowlisted names, certificate expiry within 21 days.

## 10. Open items

| ID | Open item | Needed for |
|---|---|---|
| OPEN-NW-01 | Decided 2026-10-07: AWS, ap-southeast-5 primary, ap-southeast-1 DR (§2a, DEC-68) | — |
| OPEN-NW-02 | Cellular connectivity provider and SIM management for sites without Wi-Fi | Field connectivity |
| OPEN-NW-03 | Decided 2026-10-07: HQ admin app only from the company network (§3, DEC-68); the company must supply its office egress addresses and VPN egress addresses | — |
| OPEN-NW-04 | Decided 2026-10-07: customer office firewall requirements in §5a | — |

Additional contracts for current version 0.30.0: Read IR01–139 in the [Re-review Correction Contracts](review-resolution-contracts.md). They take priority over older text on the same topic; follow IR72 for conflict precedence.
