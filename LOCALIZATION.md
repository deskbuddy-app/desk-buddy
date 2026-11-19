# Simplified Chinese localization candidate

This contribution is based on Desk Buddy v1.3, commit `7245f1a344efd0584246b96f4d0023ce2666e2c3`.
It localizes the Mac distribution, including the Kindle payload and phone page that it ships.
This is a fixed Simplified Chinese candidate; it does not add a language selector.

## What changes

- Mac app-owned windows, menus, settings, task controls, permission explanations, dialogs and privacy usage descriptions.
- The bundled Kindle UI, display dates, missing-glyph fallback and Chinese display wrapping. The existing English on-screen keyboard remains.
- The bundled phone page labels, hints, accessibility text and display dates.
- Process-local Chinese font registration on macOS, plus font packaging and regression checks. No font is installed system-wide.

API and database identifiers, permission decision values, user content, input limits, networking, synchronization and update behavior retain the upstream implementation. Scope checks and differential tests cover selected paths; they are not a proof of complete behavioral equivalence.
Brand names, user content, historical release notes, the English word clock and some diagnostics remain in English.

## Font provenance and licensing

The two `SourceHanSansSC-Regular.otf` copies are identical, unmodified Source Han Sans SC Regular **2.005R** files from Adobe:

- Source: https://github.com/adobe-fonts/source-han-sans/blob/2.005R/OTF/SimplifiedChinese/SourceHanSansSC-Regular.otf
- Font SHA-256: `f1d8611151880c6c336aabeac4640ef434fa13cbfbf1ffe82d0a71b2a5637256`
- File size: 16,529,832 bytes per copy. Git stores their identical contents as one blob; distribution bundles can contain multiple copies.
- License: SIL Open Font License 1.1, retained with Adobe's copyright and reserved font name notice in each adjacent `OFL-SourceHanSansSC.txt`.
- License SHA-256: `fcac737e761ec63dbfbdce11030a1780161920d80315edba9c8beff1c2bac5a2`.

The project's MIT license remains unchanged. The font retains its own OFL license.
The full font is intentionally bundled without subsetting; download size and Kindle memory/readability need maintainer and device review.

## Build and verify

Use the existing Go, Swift and locked npm dependencies; no production dependency or framework is added.
After installing the existing dependencies in `worker/`, run from the repository root:

```sh
python3 localization/build-candidate.py
python3 localization/verify.py --out /tmp/desk-buddy-localization-verification
python3 localization/check-scope.py
go run localization/tools/scope.go
```

The build writes packages under `menubar/dist` and does not install or launch the production app.
The helper selects an available macOS SDK; `SDKROOT` can override it. Preview verification requires a compatible SDK via `SDKROOT` or the helper's documented default.
The differential checks require the v1.3 baseline commit in local Git history; a source ZIP alone does not carry that history.
See [the Chinese development notes](localization/README.zh-Hans.md) for details.

## Validation boundaries

Host-side tests, offline previews and builds are distinct from device acceptance.
A previous local Mac installation and launch received an agent-observed smoke check; this is not human acceptance.
Intel execution, native Chinese IME composition, real calendar/music permissions, complete three-device synchronization, update/recovery drills and Kindle touch/refresh/performance are not claimed as verified by this contribution.
The candidate keeps the upstream app identity and updater; it is not an isolated side-by-side installation and an upstream update may replace the localization.
