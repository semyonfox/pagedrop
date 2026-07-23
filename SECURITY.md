# Security

## Reporting vulnerabilities

Please report vulnerabilities privately through GitHub's security advisory
feature rather than opening a public issue.

## Security model

Seol treats every uploaded page as an untrusted JavaScript application.
Anyone who possesses a public page URL can view it; unguessable URLs are not a
substitute for authentication.

- Use a dedicated content hostname with no sensitive cookies.
- Never scope API or administration cookies to a parent domain shared with the
  content hostname.
- Keep the bearer token out of uploaded content and browser-side JavaScript.
- Do not upload secrets, credentials, or confidential personal information.
- Put the API behind HTTPS and keep the local origin bound to loopback or a
  private container network.

ZIP uploads reject absolute paths, traversal paths, backslashes, symbolic
links, special files, excessive file counts, and excessive extracted sizes.
