# Release

Two artefacts are attached to every tagged GitHub Release:

| Artefact | How it is built | Status |
|---|---|---|
| `parley-<version>-arm64.apk` | Signed release APK, built on Linux in the Android builder image (`app/android-builder.Dockerfile`), signed with the keystore from repository secrets | Signing secrets configured 2026-10-28 (falls back to a debug APK if they are absent) |
| `parley-<version>-unsigned.ipa` | Unsigned IPA, built on a macOS runner with `tauri ios build --no-sign` | Ready; install via AltStore/SideStore |

Both are produced by `.github/workflows/release.yml`.

## Checklist

No secret, keystore or certificate is in the repository.

- [x] Android release keystore and the four `ANDROID_*` repository secrets created 2026-10-28. The keystore and its password are held outside the repository (maintainer's `%USERPROFILE%\.parley-release\`); back them up.
- [ ] **PENDING-USER**: create an Apple Developer account and the four `APPLE_*` secrets, only if a signed IPA is ever wanted (procedure below). Not required for the unsigned IPA.
- [x] Tag `v0.0.1` and the GitHub Release published 2026-10-28, with explicit user authorisation. Pushing a tag is otherwise human-gated.
- [ ] **PENDING-USER**: install the artefacts on a real phone (APK via `adb`/sideload, IPA via AltStore/SideStore).

Until the Android secrets exist the workflow still runs: it builds a **debug-signed** APK named `parley-<version>-arm64-debug.apk`, so a release is never blocked by missing secrets. Debug and release APKs have different signing keys — uninstall one before installing the other.

## Trigger

- Push a tag matching `v*` → build both artefacts, then create/update the GitHub Release.
- Actions → **Release** → *Run workflow* → build both artefacts only. The release job is skipped because the ref is a branch. Use this as a dry run.

## Android signing key

The key is **user-held**. Generate it once, back it up offline (password manager or encrypted drive), and never commit it. `tmp/`, `.env` and `node_modules/` are gitignored; still, do not put a real keystore anywhere inside the repository.

1. Generate the release keystore. `keytool` ships with any JDK 17+; run the commands in WSL/Linux, or use the JDK's `keytool.exe` on Windows:

   ```bash
   mkdir -p ~/parley-keys
   keytool -genkeypair \
     -keystore ~/parley-keys/parley-release.jks \
     -alias parley \
     -keyalg RSA -keysize 4096 -validity 10000
   keytool -list -v -keystore ~/parley-keys/parley-release.jks | grep -E 'Alias|SHA256'
   ```

   Use a strong store password, a strong key password (they may be the same), and record both. Losing them means installed apps can never be updated with the same identity; users must uninstall and reinstall.

2. Base64-encode the keystore:

   ```bash
   base64 -w0 ~/parley-keys/parley-release.jks > ~/parley-keys/parley-release.jks.b64
   ```

   macOS: `base64 -i ~/parley-keys/parley-release.jks -o ~/parley-keys/parley-release.jks.b64`.
   PowerShell: `[Convert]::ToBase64String([IO.File]::ReadAllBytes("$HOME\parley-keys\parley-release.jks")) | Set-Content "$HOME\parley-keys\parley-release.jks.b64"`.

3. Store the four repository secrets (Settings → Secrets and variables → Actions, or `gh`):

   ```bash
   gh secret set ANDROID_KEYSTORE_BASE64 < ~/parley-keys/parley-release.jks.b64
   gh secret set ANDROID_KEYSTORE_PASSWORD   # paste the store password
   gh secret set ANDROID_KEY_ALIAS --body parley
   gh secret set ANDROID_KEY_PASSWORD        # paste the key password
   ```

   The workflow references these names and nothing else. Passwords are passed to the container as environment variables and never echoed.

4. Delete the `.b64` file after uploading it (`rm ~/parley-keys/parley-release.jks.b64`). Keep the `.jks` and passwords off the repository.

The workflow then builds the release APK, zip-aligns it, signs it with `apksigner`, and runs `apksigner verify --print-certs`, a package-id check and a `RECORD_AUDIO` permission check before uploading it. The signing certificate's SHA-256 can be compared with the `keytool` output from step 1.

## Apple signing — PENDING-USER, optional

The default release ships an **unsigned** IPA, which is what AltStore/SideStore need: those tools re-sign the app on the device with the user's own Apple ID. No Apple Developer account is required for that path.

If a directly signed IPA is wanted later, adding a certificate is a secrets-only change — the workflow already contains the import step, guarded by the presence of `APPLE_CERTIFICATE_BASE64`:

1. Enroll in the Apple Developer Program and sign in to <https://developer.apple.com/account>.
2. Register the App ID `app.parley.translate` (the `identifier` in `app/src-tauri/tauri.conf.json`).
3. Create a **Development** certificate and register the test devices' UDIDs.
4. Export the certificate **with its private key** as a password-protected `.p12`.
5. Create a development provisioning profile for the App ID that includes those devices, and download the `.mobileprovision`.
6. Encode both and set the secrets:

   ```bash
   base64 -w0 parley-development.p12 > parley-development.p12.b64
   base64 -w0 parley.mobileprovision > parley.mobileprovision.b64
   gh secret set APPLE_CERTIFICATE_BASE64 < parley-development.p12.b64
   gh secret set APPLE_CERTIFICATE_PASSWORD     # the .p12 export password
   gh secret set APPLE_PROVISIONING_PROFILE_BASE64 < parley.mobileprovision.b64
   gh secret set APPLE_TEAM_ID --body XXXXXXXXXX  # 10-character Team ID
   ```

7. Delete the `.b64` files. The workflow then imports the certificate into a temporary keychain, installs the profile, and builds without `--no-sign` using `--export-method debugging` (development-signed IPA for sideloading). The artefact is named `parley-<version>.ipa` instead of `-unsigned.ipa`.

For TestFlight/App Store distribution, change `--export-method debugging` to `app-store-connect` in the workflow; that is beyond the sideloading release.

## Cutting a release — human-gated

Only after the user explicitly authorises a push:

1. Bump the version in all four places:
   - `package.json` → `version`
   - `app/package.json` → `version`
   - `app/src-tauri/Cargo.toml` → `[package] version` (then rebuild once so `Cargo.lock` records it)
   - `app/src-tauri/tauri.conf.json` → `version`
2. Run the CI checks locally: `bun install --frozen-lockfile && bun run lint && bun run test && bun run smoke`.
3. Commit, then tag and push **only the tag** (or push the commit and the tag once authorised):

   ```bash
   git tag -a v0.0.1 -m "Parley 0.0.1"
   git push origin v0.0.1
   ```

   Pushing the tag is what triggers the Release workflow and publishes the GitHub Release. The agent must not do this step.
4. Watch the **Release** workflow. On success the GitHub Release contains the APK, the IPA, and a `.sha256` file for each. Release notes link to the sideloading section below.
5. If the build jobs succeeded but the release job failed, re-run the failed jobs. The publish step uploads with `--clobber`, so re-running is safe. It creates the release if it does not exist yet.

A manual **Run workflow** on a branch exercises both builds without publishing anything; download the artefacts from the run page.

## Sideloading

### Android

```bash
adb install -r parley-0.0.1-arm64.apk
```

Or copy the APK to the phone and tap it; allow "Install unknown apps" for the file manager or browser when prompted. If an older build with a different signing key is installed, uninstall it first (`adb uninstall app.parley.translate`). The APK declares `RECORD_AUDIO`; the permission is requested on first use of the microphone.

### iOS unsigned IPA (AltStore / SideStore)

An unsigned IPA cannot be installed on a stock iPhone directly. It must be re-signed on or for the device with the user's Apple ID:

- **AltStore** (<https://altstore.io>): install AltServer on Windows/macOS, connect the iPhone once by USB and install AltStore, then on the phone open **My Apps → +** and choose the downloaded `parley-<version>-unsigned.ipa`. AltServer signs it with the Apple ID configured there. Free Apple IDs: the app expires after 7 days and must be refreshed in AltStore; limits of 3 apps and 10 app IDs per week apply.
- **SideStore** (<https://sidestore.io>): install SideStore using its pairing-file procedure, keep its VPN profile enabled, then add the IPA from the Files app.
- **Sideloadly** is a desktop alternative that signs and installs in one step.
- After installing, trust the developer: **Settings → General → VPN & Device Management → Developer App → Trust**.

Because the IPA is re-signed per user, it has no distribution certificate, no App Store validation and no push/entitlement guarantees. That is expected for this release channel.

## Local builds

- Android debug APK (WSL `arch`, Docker): `bun run build:apk`. It builds `parley-android-builder:local` from `app/android-builder.Dockerfile` if missing, generates `tmp/parley-debug.keystore`, and writes `tmp/parley-<version>-arm64-debug.apk`.
- The first build on a fresh machine pulls the Rust and Bun images and downloads the Android SDK/NDK (about 2 GB) and takes several minutes. The workflow rebuilds the image on every run; move it to GHCR if releases become frequent.
- iOS builds require macOS with Xcode; `tauri ios build --no-sign` cannot run on Windows or Linux.

## Secrets and repository hygiene

- No keystore, certificate, provisioning profile, token or password is committed. The workflow references secrets by name only.
- `ANDROID_*` and `APPLE_*` are the complete set of secret names, listed in the procedures above. Nothing else is read.
- Signing steps are skipped when their secrets are absent, so the workflow is runnable before any account exists.
