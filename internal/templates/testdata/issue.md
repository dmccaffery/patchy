<!-- patchy:manifest v1 {"source":"ghas","advisories":["CWE-79","CVE-2026-1234"],"rule_id":"js/reflected-xss","title":"Reflected cross-site scripting","description":"Directly writing user input to the page allows XSS.\n\nSanitize all user input.","severity":"high","alerts":[{"number":7,"url":"https://github.com/acme/shop/security/code-scanning/7","locations":[{"path":"src/render.js","start_line":42,"end_line":44}]}]} -->

## Reflected cross-site scripting

|            |                              |
| ---------- | ---------------------------- |
| Source     | `ghas`                |
| Rule       | `js/reflected-xss`                |
| Severity   | high                |
| Advisories | CWE-79, CVE-2026-1234    |

### Description

Directly writing user input to the page allows XSS.

Sanitize all user input.

### Alerts

| Alert | Locations |
| ----- | --------- |
| [#7](https://github.com/acme/shop/security/code-scanning/7) | `src/render.js:42` |

---

_This issue is managed by patchy; the body is owned by the accumulator and is rewritten as further alerts of the
same finding type arrive. Context, classification, and remediation land as comments below._
