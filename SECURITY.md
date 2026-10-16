# Security

golib is a library: a vulnerability in it is a vulnerability in every program that uses it, among them
[tadl](https://github.com/womat/tadl), [s0meter](https://github.com/womat/s0meter),
[smartmeter](https://github.com/womat/smartmeter), [modbusgateway](https://github.com/womat/modbusgateway) and
[relayhat](https://github.com/womat/relayhat). Its `web` package decides who may call a service: bearer tokens
stored as SHA-256 hashes in a token file, an IP filter, CORS. Reports of security issues are taken seriously.

## Reporting a vulnerability

Please **do not open a public issue**. Report it privately through GitHub instead:
**Security → Report a vulnerability** ([direct link](https://github.com/womat/golib/security/advisories/new)).

Helpful details:

- the affected version (the `github.com/womat/golib/v2` line in your `go.mod`) and package
- steps to reproduce, ideally a small test
- what an attacker could achieve with it

You will usually get an answer within a week. golib is a spare-time project, so no fixed response time can be
promised.

## Supported versions

Security fixes are made for the latest v2 release only. Update with `go get github.com/womat/golib/v2@latest`.
v1 receives no fixes; the packages that moved out of it are maintained in their own repositories
([gpio](https://github.com/womat/gpio), [manchester](https://github.com/womat/manchester),
[mqtt](https://github.com/womat/mqtt), [keyvalue](https://github.com/womat/keyvalue)).
