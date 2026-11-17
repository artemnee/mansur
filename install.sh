#!/bin/sh
# Installs Mansur from GitHub Releases into ~/.local/bin (no sudo).
#
#   curl -fsSL https://raw.githubusercontent.com/artemnee/mansur/main/install.sh | sh
#
# What it does: picks the build for your Mac (Apple Silicon or Intel), downloads it from the
# release, checks it against SHA256SUMS and puts the `mansur` binary into ~/.local/bin.
# It does not use sudo and does not edit your shell config.
#
# Options (environment variables):
#   MANSUR_VERSION=v0.1.0          install a specific release instead of the latest
#   MANSUR_INSTALL_DIR=/some/dir   install somewhere else
#
# Uninstall: mansur stop; rm ~/.local/bin/mansur

set -eu

repo="artemnee/mansur"

say() { printf '%s\n' "$*"; }
fail() {
	printf 'mansur install: %s\n' "$*" >&2
	exit 1
}

# Everything runs inside main, called on the last line: a partially downloaded script does nothing.
main() {
	[ "$(uname -s)" = "Darwin" ] || fail "Mansur only runs on macOS"

	case "$(uname -m)" in
	arm64) arch=arm64 ;;
	x86_64)
		arch=amd64
		# An Intel shell running under Rosetta on Apple Silicon still gets the native build.
		if [ "$(sysctl -in sysctl.proc_translated 2>/dev/null || echo 0)" = "1" ]; then arch=arm64; fi
		;;
	*) fail "unsupported CPU: $(uname -m)" ;;
	esac

	version="${MANSUR_VERSION:-latest}"
	if [ "$version" = "latest" ]; then
		base="https://github.com/$repo/releases/latest/download"
	else
		base="https://github.com/$repo/releases/download/$version"
	fi
	base="${MANSUR_BASE_URL:-$base}"
	bin_dir="${MANSUR_INSTALL_DIR:-$HOME/.local/bin}"
	archive="mansur-darwin-$arch.tar.gz"

	tmp="$(mktemp -d)"
	trap 'rm -rf "$tmp"' EXIT INT TERM

	say "Downloading $archive ($version)..."
	curl -fsSL --proto '=https,file' -o "$tmp/$archive" "$base/$archive" || fail "could not download $base/$archive"
	curl -fsSL --proto '=https,file' -o "$tmp/SHA256SUMS" "$base/SHA256SUMS" || fail "could not download SHA256SUMS"

	say "Checking the checksum..."
	(cd "$tmp" && grep " $archive\$" SHA256SUMS | shasum -a 256 -c - >/dev/null) ||
		fail "checksum mismatch for $archive: the download is corrupted or was tampered with"

	tar -xzf "$tmp/$archive" -C "$tmp"
	mkdir -p "$bin_dir"
	mv "$tmp/mansur" "$bin_dir/mansur"
	chmod 755 "$bin_dir/mansur"
	say "Installed $bin_dir/mansur"

	case ":$PATH:" in
	*":$bin_dir:"*) say "Run: mansur" ;;
	*)
		say ""
		say "$bin_dir is not in your PATH. Add it once:"
		say "  echo 'export PATH=\"$bin_dir:\$PATH\"' >> ~/.zshrc && source ~/.zshrc"
		say "Then run: mansur"
		;;
	esac
}

main "$@"
