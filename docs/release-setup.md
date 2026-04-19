# Release-time distribution setup

Goreleaser publishes to several channels on every `v*` tag push. Most require a one-time external account + a GitHub Actions secret. This page is the checklist.

All secrets live under **Settings → Secrets and variables → Actions** on the repo; the `release` workflow also pins `environment: production` so you can scope these to the production environment if you prefer.

## GitHub Container Registry (ghcr.io)

Works out of the box — uses the built-in `GITHUB_TOKEN`. No extra setup needed.

Package will appear at <https://github.com/Loupeznik/kubeconfig-manager/pkgs/container/kubeconfig-manager> after the first tagged release.

Images are signed keyless via cosign + GitHub OIDC. Verify:

```sh
cosign verify ghcr.io/loupeznik/kubeconfig-manager:<tag> \
  --certificate-identity-regexp='https://github.com/Loupeznik/kubeconfig-manager/.github/workflows/release.yml@.*' \
  --certificate-oidc-issuer='https://token.actions.githubusercontent.com'
```

## Homebrew tap

The short answer: yes, an **empty public repo + a PAT** is the whole setup. Homebrew doesn't require any central registration for third-party taps — any repo named `homebrew-<suffix>` works. The pieces and the two gotchas:

1. **Repo name must be `Loupeznik/homebrew-tap`** (the `homebrew-` prefix is mandatory — `brew tap loupeznik/tap` drops the prefix and looks for `homebrew-tap` under the `loupeznik` org). Make it **public**. Leave it empty; goreleaser creates the first commit.
2. **Personal access token**. Create a **classic** PAT (fine-grained tokens don't always cover the push-to-another-repo case goreleaser needs). Scope: `repo`. Expiry ≥ 1 year. Add it as the `HOMEBREW_TAP_TOKEN` Actions secret on the kubeconfig-manager repo.
3. (Optional but recommended) Add a short `README.md` in the tap repo so visitors understand what it is. goreleaser doesn't touch the README after creating it, so a one-paragraph description is a one-time effort.

On release, goreleaser:

- Generates `Formula/kcm.rb` under the `Formula/` directory of the tap repo (we pin `directory: Formula` in `.goreleaser.yaml`).
- Commits + pushes as `goreleaserbot`.
- Attaches the archive URL + SHA-256 of the tarball goreleaser just built for the target arch/OS.

Users then install either way:

```sh
brew install loupeznik/tap/kcm                 # one-liner
# or
brew tap loupeznik/tap && brew install kcm     # two-step
```

The formula installs the binary + man pages under `man1/`, and generates bash/zsh/fish completions via `kcm completion` at install time.

Gotchas to know about:

- **First release will fail with `HOMEBREW_TAP_TOKEN` unset.** Set the secret before tagging, or the `brews:` stage errors at the push step. The rest of the release (GitHub release, ghcr, AUR) still completes because goreleaser runs them in parallel.
- **`brew audit` is not enforced.** That's only for homebrew-core submissions. Third-party taps can ship anything that parses as a Ruby formula.
- **The tap repo doesn't need to exist before the first release** — goreleaser will push the first commit regardless. But creating it yourself means you can set up branch protection, a README, and a topic (`homebrew-tap`) upfront.

## AUR (Arch User Repository)

One-time:

1. Create an AUR account at <https://aur.archlinux.org/register>.
2. Register the package name by pushing an initial empty commit with a `.SRCINFO` — or let goreleaser create it on the first release (the AUR creates the repo on first SSH push).
3. Generate a dedicated SSH keypair (do **not** reuse your regular SSH key):

    ```sh
    ssh-keygen -t ed25519 -N '' -f aur_key -C 'goreleaser-aur'
    ```

4. Add the **public** key (`aur_key.pub`) under <https://aur.archlinux.org/account/> → *My Account* → *SSH Public Key*.
5. Add the **private** key (`aur_key` contents) as the `AUR_KEY` Actions secret on this repo.

On release:

- Goreleaser pushes a `PKGBUILD` + `.SRCINFO` to `ssh://aur@aur.archlinux.org/kubeconfig-manager-bin.git`.
- Users install via `yay -S kubeconfig-manager-bin` (or any other AUR helper).
- Package installs the binary, man pages, and bash/zsh/fish completions.

## Snap Store — postponed

Snap publishing is deliberately **not** wired in the current `.goreleaser.yaml`. The `kubeconfig-manager` name was blocked during store registration and needs a dispute / clarification round with the snap store team before we can proceed. Until then, we don't generate `.snap` artifacts on release.

When we come back to it, the work is:

1. Resolve the name collision (either clear `kubeconfig-manager` via <https://forum.snapcraft.io/c/store-requests/19> or fall back to `kcm` — which may also be taken; check with `snapcraft register --dry-run`).
2. Re-add the `snapcrafts:` block to `.goreleaser.yaml` — `id: kcm`, `ids: [kcm]`, `confinement: classic`, `grade: stable`, `license: Apache-2.0`, with a single `apps.kcm` entry. Reference: <https://goreleaser.com/customization/package/snapcraft/>.
3. Re-add the `snapcraft` install step + `SNAPCRAFT_STORE_CREDENTIALS` env to `release.yml`.
4. Request classic-confinement approval via the forum link above before the first published release — classic confinement is needed because kcm reads `~/.kube`, exec's `kubectl` / `helm`, and writes to the user's shell rc.

## Secrets quick-reference

| Secret | Used by | Created via |
| --- | --- | --- |
| `HOMEBREW_TAP_TOKEN` | `brews:` | Classic PAT with `repo` scope |
| `AUR_KEY` | `aurs:` | `ssh-keygen`, public key on AUR |
| `GITHUB_TOKEN` | ghcr, release notes | Built-in (no setup) |

## Before the next tag

- Confirm every secret above is present and not expired.
- `goreleaser check` locally (or rely on the `goreleaser config check` CI job) — it validates all four package manager blocks.
- Dry-run end-to-end: `goreleaser release --snapshot --clean --skip=publish`.
