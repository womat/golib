# Security

golib is a library: a vulnerability in it is a vulnerability in every program that uses it, among them
[relayhat](https://github.com/womat/relayhat) and [s0meter](https://github.com/womat/s0meter). Its packages
handle API keys and JWTs (`web`, `jwt_util`), encryption (`crypt`) and GPIO access (`gpio`). Reports of security
issues are taken seriously.

## Reporting a vulnerability

Please **do not open a public issue**. Report it privately through GitHub instead:
**Security → Report a vulnerability** ([direct link](https://github.com/womat/golib/security/advisories/new)).

Helpful details:

- the affected version (the `github.com/womat/golib` line in your `go.mod`) and package
- steps to reproduce, ideally a small test
- what an attacker could achieve with it

You will usually get an answer within a week. golib is a spare-time project, so no fixed response time can be
promised.

## Supported versions

Security fixes are made for the latest release only. Update with `go get github.com/womat/golib@latest`.
