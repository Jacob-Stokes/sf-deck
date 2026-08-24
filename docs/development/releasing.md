# Releasing sf-deck

Tagged releases are built by GoReleaser. Linux packages and portable archives
receive GitHub build-provenance attestations. Apple signing and notarisation is
optional until the project can fund an Apple Developer Program membership.
When all Apple credentials are present, macOS executables are signed and
notarised before they are archived or published to the Homebrew tap.

Every release requires:

- `HOMEBREW_TAP_GITHUB_TOKEN`

Apple signing additionally requires all five of these secrets:

- `MACOS_SIGN_P12`: base64-encoded Developer ID Application `.p12`
- `MACOS_SIGN_PASSWORD`: password for the `.p12`
- `MACOS_NOTARY_KEY`: base64-encoded App Store Connect API `.p8`
- `MACOS_NOTARY_KEY_ID`: App Store Connect API key ID
- `MACOS_NOTARY_ISSUER_ID`: App Store Connect issuer UUID

The Apple credentials require an active Apple Developer Program membership.
Keep them only in GitHub Actions secrets. Never add certificate or key files to
the repository.

The workflow accepts either all five Apple secrets or none. A partial Apple
configuration fails closed. After adding or rotating them, manually run the
**Release** workflow. The manual path parses the certificate and notarisation
key and verifies Homebrew tap write access without creating a release.

Before pushing a tag, add a non-empty
`.github/release-notes/<tag>.md`, run the full local checks, and confirm the tag
points at the intended commit. After publishing, verify a downloaded artifact:

```sh
gh attestation verify sf-deck_<version>_<os>_<arch>.tar.gz \
  --repo Jacob-Stokes/sf-deck
```

On macOS, `codesign --verify --verbose=2 <path-to-sf-deck>` verifies the code
signature. Gatekeeper performs the online notarisation-ticket check when the
downloaded executable is first opened.
