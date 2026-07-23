---
name: pagedrop
description: Publish HTML files or static-site directories with PageDrop and return shareable URLs. Use for generated reports, dashboards, docs, demos, or other static pages the user asks to host, deploy, upload, or share.
---

# PageDrop

Publish only when requested; this is an external side effect.

1. Require `pagedrop` on `PATH`.
2. Accept one `.html`/`.htm` file, or a directory/ZIP with root `index.html`.
3. For framework source, inspect its scripts/docs; when useful, run its existing
   production build and upload the static output (`dist/`, `build/`, etc.). Never
   invent an unknown build command.
4. Check text for obvious credentials or sensitive data; stop and warn if found.
5. Preserve relative assets (`assets/app.js`, not `/assets/app.js`).
6. Publish and return the printed URL:

```bash
pagedrop upload --quiet PATH
# Optional: --expires 7d --title "Title"
```

Omit `--expires` for the one-day default unless the user requests another
duration; seven days is the maximum. Publishing is anonymous. If the server is
not configured, ask the user to run:

```bash
pagedrop configure --server URL
```

Management still requires the instance's admin token. Never print it or the
configuration. Manage only when requested:

```bash
pagedrop list
pagedrop stats
pagedrop info PAGE_ID
pagedrop replace PAGE_ID PATH
pagedrop delete PAGE_ID
```
