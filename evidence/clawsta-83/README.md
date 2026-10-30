# CLAWSTA-83 / B1 LAN access evidence (sanitized)

The CLAWSTA-80 / C1 deployment originally allowed its web entry (TCP 13001) only
from the verifying laptop `192.168.3.162` and that laptop's Tailscale address
`100.102.214.78`. B1 widened the same named rule to the whole `192.168.3.0/24`
LAN, so any device on that network can open http://192.168.3.49:13001 without a
per-device rule. The API and PostgreSQL were not touched and stay loopback-only.

Only names, addresses, ports and status codes are recorded here: no credentials,
login codes, cookies, tokens or application data.

- `01-lan-firewall.json` — rule readback after the change (enabled, inbound,
  allow, TCP 13001, remote addresses in mask form), listener isolation on the
  deployment host, the LAN HTTP results from the client PC, and the exact
  widen/readback/restore commands.

Boundary: one client PC was used, so the subnet-wide scope is proven by the rule
readback rather than a per-device matrix. Company-network reachability and public
internet access were out of scope and were not opened. The runbook lives in
[../../docs/deployment-environment-issues.md](../../docs/deployment-environment-issues.md)
(section 13) and [../../docs/windows-native.md](../../docs/windows-native.md).
